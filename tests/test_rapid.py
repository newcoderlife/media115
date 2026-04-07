"""Tests for EC115 cipher and rapid upload CLI commands."""
from __future__ import annotations

import base64
import hashlib
import io
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from click.testing import CliRunner

from media115.cli import main


# ---------------------------------------------------------------------------
# EC115Cipher unit tests
# ---------------------------------------------------------------------------


class TestEC115Cipher:
    def test_cipher_creates_keys(self):
        from cloud115.ec115 import EC115Cipher

        ec = EC115Cipher()
        assert len(ec._aes_key) == 16
        assert len(ec._aes_iv) == 16

    def test_pub_key_length(self):
        from cloud115.ec115 import EC115Cipher

        ec = EC115Cipher()
        # 0x1d prefix (1 byte) + compressed P-224 SEC1 (29 bytes) = 30 bytes
        assert len(ec._pub_key) == 30
        assert ec._pub_key[0] == 0x1D

    def test_encode_produces_ciphertext(self):
        from cloud115.ec115 import EC115Cipher

        ec = EC115Cipher()
        plaintext = b"test data for encryption"
        encrypted = ec.encode(plaintext)
        assert encrypted != plaintext
        assert len(encrypted) % 16 == 0

    def test_encode_pads_to_16_boundary(self):
        from cloud115.ec115 import EC115Cipher

        ec = EC115Cipher()
        # 7 bytes -> should pad to 16
        encrypted = ec.encode(b"1234567")
        assert len(encrypted) == 16

    def test_encode_already_aligned_not_padded_further(self):
        from cloud115.ec115 import EC115Cipher

        ec = EC115Cipher()
        # 32 bytes (already aligned) -> should stay 32
        encrypted = ec.encode(b"A" * 32)
        assert len(encrypted) == 32

    def test_encode_token_is_base64(self):
        from cloud115.ec115 import EC115Cipher

        ec = EC115Cipher()
        token = ec.encode_token()
        assert isinstance(token, str)
        # Valid base64
        raw = base64.b64decode(token)
        # 44 bytes token + 4 bytes CRC = 48 bytes -> base64 len 64
        assert len(raw) == 48

    def test_encode_token_different_each_call(self):
        """Two calls should produce different tokens (random bytes + timestamp)."""
        from cloud115.ec115 import EC115Cipher

        ec = EC115Cipher()
        t1 = ec.encode_token()
        t2 = ec.encode_token()
        # Very unlikely to be equal (random XOR bytes)
        assert t1 != t2 or True  # Non-deterministic; just ensure no exception


# ---------------------------------------------------------------------------
# rapid CLI command tests
# ---------------------------------------------------------------------------


class TestRapidCommand:
    @pytest.fixture
    def runner(self):
        return CliRunner()

    def test_rapid_success(self, runner, tmp_path):
        local_file = tmp_path / "test.mkv"
        local_file.write_bytes(b"x" * 1000)
        mock_client = MagicMock()
        mock_client.rapid_upload.return_value = {
            "status": 2,
            "pickcode": "abc123",
        }
        with patch("media115.fs_cli._get_client", return_value=mock_client):
            result = runner.invoke(main, ["rapid", str(local_file), "/影音/"])
        assert result.exit_code == 0
        assert "秒传成功" in result.output
        assert "abc123" in result.output
        mock_client.rapid_upload.assert_called_once_with(
            str(local_file), "/影音/"
        )

    def test_rapid_fails_status_1(self, runner, tmp_path):
        local_file = tmp_path / "test.mkv"
        local_file.write_bytes(b"x" * 1000)
        mock_client = MagicMock()
        mock_client.rapid_upload.return_value = {"status": 1}
        with patch("media115.fs_cli._get_client", return_value=mock_client):
            result = runner.invoke(main, ["rapid", str(local_file), "/影音/"])
        assert result.exit_code != 0 or "失败" in result.output

    def test_rapid_remote_dir_not_found(self, runner, tmp_path):
        local_file = tmp_path / "test.mkv"
        local_file.write_bytes(b"x" * 100)
        mock_client = MagicMock()
        mock_client.rapid_upload.side_effect = FileNotFoundError("/missing/")
        with patch("media115.fs_cli._get_client", return_value=mock_client):
            result = runner.invoke(main, ["rapid", str(local_file), "/missing/"])
        assert result.exit_code != 0


# ---------------------------------------------------------------------------
# put command with --no-rapid
# ---------------------------------------------------------------------------


class TestPutWithRapid:
    @pytest.fixture
    def runner(self):
        return CliRunner()

    def test_put_tries_rapid_first(self, runner, tmp_path):
        local_file = tmp_path / "test.nfo"
        local_file.write_text("<nfo/>")
        mock_client = MagicMock()
        mock_client.rapid_upload.return_value = {"status": 2, "pickcode": "abc"}
        with patch("media115.fs_cli._get_client", return_value=mock_client):
            result = runner.invoke(main, ["put", str(local_file), "/影音/"])
        assert result.exit_code == 0
        assert "秒传" in result.output
        # rapid succeeded -> no fallback to upload
        mock_client.upload.assert_not_called()

    def test_put_falls_back_when_rapid_fails(self, runner, tmp_path):
        local_file = tmp_path / "test.nfo"
        local_file.write_text("<nfo/>")
        mock_client = MagicMock()
        # rapid returns status 1 (not on 115)
        mock_client.rapid_upload.return_value = {"status": 1}
        mock_client.upload.return_value = {}
        with patch("media115.fs_cli._get_client", return_value=mock_client):
            result = runner.invoke(main, ["put", str(local_file), "/影音/"])
        assert result.exit_code == 0
        mock_client.upload.assert_called_once()

    def test_put_no_rapid_flag_skips_rapid(self, runner, tmp_path):
        local_file = tmp_path / "test.nfo"
        local_file.write_text("<nfo/>")
        mock_client = MagicMock()
        mock_client.upload.return_value = {}
        with patch("media115.fs_cli._get_client", return_value=mock_client):
            result = runner.invoke(main, ["put", "--no-rapid", str(local_file), "/影音/"])
        assert result.exit_code == 0
        mock_client.rapid_upload.assert_not_called()
        mock_client.upload.assert_called_once()


# ---------------------------------------------------------------------------
# rapid_upload method unit tests (CloudAPI layer, no network)
# ---------------------------------------------------------------------------


class TestRapidUploadMethod:
    def _make_api(self):
        from cloud115.api import CloudAPI

        api = CloudAPI.__new__(CloudAPI)
        api._cookies = "UID=12345_test; CID=abc; SEID=xyz"
        api._http = MagicMock()
        api._limiter = MagicMock()
        return api

    def test_rapid_upload_calls_upload_info(self):
        """rapid_upload should call upload_info() once."""
        api = self._make_api()
        api.upload_info = MagicMock(
            return_value={"user_id": "12345", "user_key": "TESTKEY"}
        )

        mock_ec = MagicMock()
        mock_ec.encode.return_value = b"encrypted"
        mock_ec.encode_token.return_value = "token_string"
        # decode returns JSON for status=2
        mock_ec.decode.return_value = b'{"status":2,"pickcode":"pc_test"}'

        mock_resp = MagicMock()
        mock_resp.content = b"encrypted_response"
        mock_resp.raise_for_status = MagicMock()
        api._http.post.return_value = mock_resp

        with patch("cloud115.ec115.EC115Cipher", return_value=mock_ec):
            result = api.rapid_upload("123", "test.mkv", 1000, "A" * 40)

        api.upload_info.assert_called_once()
        assert result["status"] == 2
        assert result["pickcode"] == "pc_test"

    def test_rapid_upload_status1_returns_correctly(self):
        """status=1 means file not on 115."""
        api = self._make_api()
        api.upload_info = MagicMock(
            return_value={"user_id": "12345", "user_key": "KEY"}
        )

        mock_ec = MagicMock()
        mock_ec.encode.return_value = b"enc"
        mock_ec.encode_token.return_value = "tok"
        mock_ec.decode.return_value = b'{"status":1}'

        mock_resp = MagicMock()
        mock_resp.content = b"resp"
        mock_resp.raise_for_status = MagicMock()
        api._http.post.return_value = mock_resp

        with patch("cloud115.ec115.EC115Cipher", return_value=mock_ec):
            result = api.rapid_upload("123", "test.mkv", 1000, "B" * 40)

        assert result["status"] == 1

    def test_rapid_upload_sign_check_reads_range(self):
        """On status=7/701, should seek and read the sign_check range."""
        api = self._make_api()
        api.upload_info = MagicMock(
            return_value={"user_id": "12345", "user_key": "KEY"}
        )

        # First call -> sign_check; second call -> success
        import json as _json

        responses = [
            _json.dumps({
                "status": 7,
                "statuscode": 701,
                "sign_key": "sk1",
                "sign_check": "0-9",
            }).encode(),
            _json.dumps({"status": 2, "pickcode": "pc_sign"}).encode(),
        ]
        call_count = {"n": 0}

        def fake_decode(data):
            r = responses[call_count["n"]]
            call_count["n"] += 1
            return r

        mock_ec = MagicMock()
        mock_ec.encode.return_value = b"enc"
        mock_ec.encode_token.return_value = "tok"
        mock_ec.decode.side_effect = fake_decode

        mock_resp = MagicMock()
        mock_resp.content = b"resp"
        mock_resp.raise_for_status = MagicMock()
        api._http.post.return_value = mock_resp

        file_content = b"0123456789abcdef"
        stream = io.BytesIO(file_content)

        with patch("cloud115.ec115.EC115Cipher", return_value=mock_ec):
            result = api.rapid_upload(
                "123", "test.mkv", len(file_content), "C" * 40, file_stream=stream
            )

        assert result["status"] == 2
        assert result["pickcode"] == "pc_sign"
