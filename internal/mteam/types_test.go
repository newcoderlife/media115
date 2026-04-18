package mteam

import (
	"encoding/json"
	"testing"

	"github.com/bytedance/gg/gconv"
)

func TestTorrent_UnmarshalJSON_StringFields(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"id": "12345", "name":"a"}`, "12345"},
		{`{"id": "", "name":"a"}`, ""},
	}
	for _, c := range cases {
		var tr Torrent
		if err := json.Unmarshal([]byte(c.in), &tr); err != nil {
			t.Fatalf("unmarshal %q: %v", c.in, err)
		}
		if tr.ID != c.want {
			t.Fatalf("got %q want %q", tr.ID, c.want)
		}
	}
}

func TestTorrent_UnmarshalJSON_BadPayload(t *testing.T) {
	var tr Torrent
	if err := json.Unmarshal([]byte(`"nope"`), &tr); err == nil {
		t.Fatal("expected error on non-object payload")
	}
}

func TestSearchResponse_Decode(t *testing.T) {
	raw := `{
		"code": "0",
		"message": "SUCCESS",
		"data": {
			"pageNumber": "1",
			"pageSize": "50",
			"total": "2",
			"totalPages": "1",
			"data": [
				{"id": "1", "name":"A", "size":"123", "numfiles":"3",
				 "status":{"seeders":"10","leechers":"1","discount":"FREE"}},
				{"id": "2", "name":"B", "size":"456", "numfiles":"5",
				 "status":{"seeders":"0","leechers":"0","discount":"NORMAL"}}
			]
		}
	}`
	var resp SearchResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data.List) != 2 {
		t.Fatalf("len=%d", len(resp.Data.List))
	}
	if resp.Data.List[0].ID != "1" || resp.Data.List[1].ID != "2" {
		t.Fatalf("ids=%q,%q", resp.Data.List[0].ID, resp.Data.List[1].ID)
	}
	if gconv.To[int64, string](resp.Data.List[0].Size) != 123 {
		t.Fatalf("size[0]=%d", gconv.To[int64, string](resp.Data.List[0].Size))
	}
	if gconv.To[int64, string](resp.Data.List[1].Size) != 456 {
		t.Fatalf("size[1]=%d", gconv.To[int64, string](resp.Data.List[1].Size))
	}
	if gconv.To[int64, string](resp.Data.List[0].Status.Seeders) != 10 {
		t.Fatalf("seeders=%d", gconv.To[int64, string](resp.Data.List[0].Status.Seeders))
	}
}
