// Package cloud115 – CachedClient: cache-first reads + write-through writes.
//
// Client wraps API and Cache into a single high-level facade. Every read
// checks the SQLite cache first (respecting TTL); every write calls the API
// then updates the cache so subsequent reads stay warm.
package cloud115

import (
	"crypto/sha1"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/logging"
)

// ── Options ───────────────────────────────────────────────────────────────────

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithCacheDir overrides the directory used for cache.db.
func WithCacheDir(dir string) ClientOption { return func(c *Client) { c.cacheDir = dir } }

// WithListingTTL overrides the directory-listing TTL (default 1 h).
func WithListingTTL(d time.Duration) ClientOption { return func(c *Client) { c.listingTTL = d } }

// WithPathTTL overrides the path→cid TTL (default 24 h).
func WithPathTTL(d time.Duration) ClientOption { return func(c *Client) { c.pathTTL = d } }

// WithLogger overrides the structured logger used by the client.
func WithLogger(l *slog.Logger) ClientOption { return func(c *Client) { c.logger = l } }

// WithStats attaches a Stats tracker to the client for call counting.
func WithStats(s *logging.Stats) ClientOption { return func(c *Client) { c.stats = s } }

// ── apiCaller ─────────────────────────────────────────────────────────────────

// apiCaller is the subset of *API used by Client. Declaring it as an interface
// allows tests to inject a stub without touching the production *API type.
type apiCaller interface {
	GetDirID(path string) (string, error)
	ListFilesAll(cid string) ([]map[string]any, error)
	Search(keyword, dirID string) ([]map[string]any, error)
	DownloadURL(pickCode, userAgent string) (string, error)
	ExportTree(dirID string) (string, error)
	Mkdir(parentID, name string) (map[string]any, error)
	Rename(fileID, newName string) (map[string]any, error)
	Delete(fileIDs []string) (map[string]any, error)
	Move(fileIDs []string, targetDirID string) (map[string]any, error)
	BatchRename(renames map[string]string) (map[string]any, error)
	UploadFile(localPath, targetDirID, filename string) (map[string]any, error)
	RapidUpload(dirID, filename string, fileSize int64, fileSHA1 string, fileStream io.ReadSeeker) (map[string]any, error)
	QRLogin(app string) error
	QRGetToken(app string) (*QRSession, error)
	QRWaitAndLogin(sess *QRSession) error
	CheckLogin() bool
	RenewCookies(app string) bool
	SaveCookies(path string) error
	GetCookies() string
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client is a high-level 115 cloud client with transparent SQLite caching.
type Client struct {
	api        apiCaller
	cache      *Cache
	listingTTL time.Duration
	pathTTL    time.Duration
	logger     *slog.Logger
	cacheDir   string
	stats      *logging.Stats
}

// NewClient creates a Client. cookies is the 115 cookie string.
func NewClient(cookies string, opts ...ClientOption) (*Client, error) {
	c := &Client{
		listingTTL: time.Hour,
		pathTTL:    24 * time.Hour,
		logger:     slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.cacheDir == "" {
		c.cacheDir = config.CacheDir()
	}

	var err error
	c.cache, err = NewCache(filepath.Join(c.cacheDir, "cache.db"))
	if err != nil {
		return nil, fmt.Errorf("open cache: %w", err)
	}

	c.api = NewAPI(cookies, c.cache, c.logger) // *API satisfies apiCaller
	return c, nil
}

// Close releases the database connection.
func (c *Client) Close() error {
	return c.cache.Close()
}

// ── helpers ───────────────────────────────────────────────────────────────────

// normalize ensures path starts with "/", strips trailing "/", empty → "/".
func normalize(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(path, "/")
}

// normalizeItem converts a raw 115 API map to an Entry.
// A file has "fid" in the raw map; a directory has "cid".
func normalizeItem(raw map[string]any) Entry {
	name, _ := raw["n"].(string)
	if name == "" {
		name, _ = raw["fn"].(string)
	}
	if name == "" {
		name, _ = raw["name"].(string)
	}

	if _, isFile := raw["fid"]; isFile {
		var nodeID string
		switch v := raw["fid"].(type) {
		case string:
			nodeID = v
		default:
			nodeID = fmt.Sprintf("%v", v)
		}
		size := int64(0)
		switch v := raw["s"].(type) {
		case float64:
			size = int64(v)
		case int64:
			size = v
		}
		pickCode, _ := raw["pc"].(string)
		return Entry{
			Name:     name,
			Type:     "file",
			NodeID:   nodeID,
			FID:      nodeID,
			Size:     size,
			PickCode: pickCode,
		}
	}

	// directory
	var cid string
	if v, ok := raw["cid"]; ok {
		cid = fmt.Sprintf("%v", v)
	} else if v, ok := raw["fid"]; ok {
		cid = fmt.Sprintf("%v", v)
	}
	return Entry{
		Name:   name,
		Type:   "dir",
		NodeID: cid,
		CID:    cid,
	}
}

// ── READ OPERATIONS ───────────────────────────────────────────────────────────

// ResolvePath resolves a cloud path to a directory cid.
// Returns "0" for root. Returns an error wrapping fs.ErrNotExist if missing.
func (c *Client) ResolvePath(path string) (string, error) {
	path = normalize(path)
	if path == "/" {
		return "0", nil
	}

	if cid, ts, ok := c.cache.GetPath(path); ok {
		age := time.Now().Unix() - ts
		if age < int64(c.pathTTL.Seconds()) {
			c.logger.Debug("CACHE HIT  resolve", "path", path, "cid", cid, "age_s", age)
			if c.stats != nil {
				c.stats.Incr("cache_hit")
			}
			return cid, nil
		}
	}

	cid, err := c.api.GetDirID(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	// 115 returns id="0" or empty string for non-existent paths.
	if cid == "" || cid == "0" {
		return "", fmt.Errorf("path not found on 115: %s", path)
	}
	c.cache.SetPath(path, cid)
	c.logger.Debug("CACHE MISS resolve", "path", path, "cid", cid)
	if c.stats != nil {
		c.stats.Incr("cache_miss")
	}
	return cid, nil
}

// ListDir lists a directory by path (cache-first).
func (c *Client) ListDir(path string) ([]Entry, error) {
	path = normalize(path)
	cid, err := c.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	return c.listDirByCID(cid, path)
}

// listDirByCID lists directory cid, returning cached entries when fresh.
func (c *Client) listDirByCID(cid, label string) ([]Entry, error) {
	if ts, ok := c.cache.GetDirTS(cid); ok {
		age := time.Now().Unix() - ts
		if age < int64(c.listingTTL.Seconds()) {
			entries := c.cache.GetDirEntries(cid)
			c.logger.Debug("CACHE HIT  list_dir", "label", label, "cid", cid, "age_s", age)
			if c.stats != nil {
				c.stats.Incr("cache_hit")
			}
			return entries, nil
		}
	}

	rawItems, err := c.api.ListFilesAll(cid)
	if err != nil {
		return nil, fmt.Errorf("list dir cid=%s: %w", cid, err)
	}
	entries := make([]Entry, 0, len(rawItems))
	for _, item := range rawItems {
		entries = append(entries, normalizeItem(item))
	}
	c.cache.SetDirListing(cid, entries)
	c.logger.Debug("CACHE MISS list_dir", "label", label, "cid", cid, "items", len(entries))
	if c.stats != nil {
		c.stats.Incr("cache_miss")
	}
	return entries, nil
}

// FindFile looks up a file by its full path and returns its metadata.
func (c *Client) FindFile(path string) (*Entry, error) {
	path = normalize(path)
	if path == "/" {
		return nil, fmt.Errorf("root is not a file")
	}

	lastSlash := strings.LastIndex(path, "/")
	parentPath := path[:lastSlash]
	if parentPath == "" {
		parentPath = "/"
	}
	filename := path[lastSlash+1:]

	parentCID, err := c.ResolvePath(parentPath)
	if err != nil {
		return nil, err
	}

	// Refresh listing if stale so we can rely on the cache entry.
	if ts, ok := c.cache.GetDirTS(parentCID); !ok || time.Now().Unix()-ts >= int64(c.listingTTL.Seconds()) {
		if _, err := c.listDirByCID(parentCID, parentPath); err != nil {
			return nil, err
		}
	}

	entry := c.cache.FindEntry(parentCID, filename)
	if entry == nil {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	if entry.Type != "file" {
		return nil, fmt.Errorf("not a file: %s", path)
	}
	entry.CID = parentCID // record parent for callers
	return entry, nil
}

// ListDirUncached bypasses the cache and fetches the listing directly from
// the API. Useful for dedup scenarios where stale cache must not be trusted.
func (c *Client) ListDirUncached(path string) ([]Entry, error) {
	path = normalize(path)
	cid, err := c.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	rawItems, err := c.api.ListFilesAll(cid)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rawItems))
	for _, item := range rawItems {
		entries = append(entries, normalizeItem(item))
	}
	return entries, nil
}

// DownloadURL returns the download URL for a pick code (direct API, no cache).
func (c *Client) DownloadURL(pickCode, userAgent string) (string, error) {
	c.logger.Debug("API download", "pick_code", pickCode)
	if c.stats != nil {
		c.stats.Incr("api")
	}
	return c.api.DownloadURL(pickCode, userAgent)
}

// StreamURL resolves path → pick code → download URL in one step.
func (c *Client) StreamURL(path, userAgent string) (string, error) {
	entry, err := c.FindFile(path)
	if err != nil {
		return "", err
	}
	return c.DownloadURL(entry.PickCode, userAgent)
}

// Search searches for keyword under path.
func (c *Client) Search(keyword, path string) ([]Entry, error) {
	cid, err := c.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	rawItems, err := c.api.Search(keyword, cid)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rawItems))
	for _, item := range rawItems {
		entries = append(entries, normalizeItem(item))
	}
	return entries, nil
}

// ExportTree exports the directory tree at path as text.
func (c *Client) ExportTree(path string) (string, error) {
	cid, err := c.ResolvePath(path)
	if err != nil {
		return "", err
	}
	return c.api.ExportTree(cid)
}

// Walk recursively lists the directory tree starting at path up to maxDepth
// levels, calling fn(entry, dirPath, depth) for every entry encountered.
func (c *Client) Walk(path string, maxDepth int, fn func(Entry, string, int) error) error {
	path = normalize(path)
	return c.walkRecursive(path, maxDepth, 0, fn)
}

func (c *Client) walkRecursive(path string, maxDepth, depth int, fn func(Entry, string, int) error) error {
	if depth > maxDepth {
		return nil
	}
	entries, err := c.ListDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := fn(e, path, depth); err != nil {
			return err
		}
		if e.Type == "dir" {
			childPath := strings.TrimRight(path, "/") + "/" + e.Name
			if err := c.walkRecursive(childPath, maxDepth, depth+1, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

// Stat returns metadata for path (file or directory).
func (c *Client) Stat(path string) (*Entry, error) {
	path = normalize(path)
	if path == "/" {
		return &Entry{Name: "/", Type: "dir", NodeID: "0", CID: "0"}, nil
	}
	// Try file first.
	if entry, err := c.FindFile(path); err == nil {
		return entry, nil
	}
	// Might be a directory.
	cid, err := c.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	name := path[strings.LastIndex(path, "/")+1:]
	return &Entry{Name: name, Type: "dir", NodeID: cid, CID: cid}, nil
}

// ── WRITE OPERATIONS ──────────────────────────────────────────────────────────

// Mkdir creates a directory at path. If parents is true, intermediate
// directories are created as needed. Returns the new directory cid.
func (c *Client) Mkdir(path string, parents bool) (string, error) {
	path = normalize(path)
	if parents {
		return c.mkdirParents(path)
	}

	lastSlash := strings.LastIndex(path, "/")
	parentPath := path[:lastSlash]
	if parentPath == "" {
		parentPath = "/"
	}
	name := path[lastSlash+1:]

	parentCID, err := c.ResolvePath(parentPath)
	if err != nil {
		return "", err
	}

	result, err := c.api.Mkdir(parentCID, name)
	if err != nil {
		return "", fmt.Errorf("mkdir %s: %w", path, err)
	}
	newCID := fmt.Sprintf("%v", result["cid"])
	if newCID == "<nil>" || newCID == "" {
		newCID = fmt.Sprintf("%v", result["aid"])
	}
	c.logger.Info("API        mkdir    "+path, "parent_cid", parentCID, "new_cid", newCID)
	if c.stats != nil {
		c.stats.Incr("api")
	}

	// write-through
	c.cache.AddEntry(parentCID, Entry{
		Name:   name,
		Type:   "dir",
		NodeID: newCID,
		CID:    newCID,
	})
	c.cache.SetPath(path, newCID)
	c.logger.Debug("WRITE-THRU add_entry", "cid", parentCID, "name", name, "type", "dir", "new_cid", newCID)
	return newCID, nil
}

func (c *Client) mkdirParents(path string) (string, error) {
	parts := []string{}
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	currentPath := ""
	currentCID := "0"
	for _, part := range parts {
		currentPath += "/" + part
		cid, err := c.ResolvePath(currentPath)
		if err == nil {
			currentCID = cid
			continue
		}
		// Directory does not exist — create it.
		result, err := c.api.Mkdir(currentCID, part)
		if err != nil {
			return "", fmt.Errorf("mkdirParents %s: %w", currentPath, err)
		}
		newCID := formatNum(result["cid"])
		if newCID == "" || newCID == "0" {
			newCID = formatNum(result["aid"])
		}
		if newCID == "" || newCID == "0" {
			return "", fmt.Errorf("mkdirParents %s: no valid cid returned", currentPath)
		}
		c.cache.AddEntry(currentCID, Entry{
			Name:   part,
			Type:   "dir",
			NodeID: newCID,
			CID:    newCID,
		})
		c.cache.SetPath(currentPath, newCID)
		c.logger.Info("API        mkdir    "+currentPath, "parent_cid", currentCID, "new_cid", newCID)
		if c.stats != nil {
			c.stats.Incr("api")
		}
		currentCID = newCID
	}
	return currentCID, nil
}

// Rename renames the file or directory at path to newName.
func (c *Client) Rename(path, newName string) error {
	path = normalize(path)
	lastSlash := strings.LastIndex(path, "/")
	parentPath := path[:lastSlash]
	if parentPath == "" {
		parentPath = "/"
	}
	oldName := path[lastSlash+1:]

	parentCID, err := c.ResolvePath(parentPath)
	if err != nil {
		return err
	}

	// Refresh if stale to ensure we have node_id.
	if ts, ok := c.cache.GetDirTS(parentCID); !ok || time.Now().Unix()-ts >= int64(c.listingTTL.Seconds()) {
		if _, err := c.listDirByCID(parentCID, parentPath); err != nil {
			return err
		}
	}

	e := c.cache.FindEntry(parentCID, oldName)
	if e == nil {
		return fmt.Errorf("not found: %s", path)
	}

	if _, err := c.api.Rename(e.NodeID, newName); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	c.logger.Info("API        rename   "+path, "node_id", e.NodeID, "new_name", newName)
	if c.stats != nil {
		c.stats.Incr("api")
	}

	// write-through
	c.cache.RenameEntry(parentCID, e.NodeID, newName)
	if e.Type == "dir" {
		c.cache.DeletePathPrefix(path)
		newPath := strings.TrimRight(parentPath, "/") + "/" + newName
		c.cache.SetPath(newPath, e.NodeID)
	}
	c.logger.Debug("WRITE-THRU rename_entry", "cid", parentCID, "node_id", e.NodeID, "new_name", newName)
	return nil
}

// Delete deletes one or more cloud paths.
func (c *Client) Delete(paths []string) error {
	type resolved struct {
		path      string
		parentCID string
		name      string
		nodeID    string
		isDir     bool
	}

	ids := make([]string, 0, len(paths))
	rslv := make([]resolved, 0, len(paths))

	for _, p := range paths {
		p = normalize(p)
		lastSlash := strings.LastIndex(p, "/")
		parentPath := p[:lastSlash]
		if parentPath == "" {
			parentPath = "/"
		}
		name := p[lastSlash+1:]

		parentCID, err := c.ResolvePath(parentPath)
		if err != nil {
			return err
		}

		e := c.cache.FindEntry(parentCID, name)
		if e == nil {
			// Refresh listing and retry.
			if _, err := c.listDirByCID(parentCID, parentPath); err != nil {
				return err
			}
			e = c.cache.FindEntry(parentCID, name)
		}

		if e == nil {
			// Try treating it as a directory itself.
			cid, err := c.ResolvePath(p)
			if err != nil {
				return fmt.Errorf("not found: %s", p)
			}
			ids = append(ids, cid)
			rslv = append(rslv, resolved{p, parentCID, name, cid, true})
			continue
		}

		ids = append(ids, e.NodeID)
		rslv = append(rslv, resolved{p, parentCID, name, e.NodeID, e.Type == "dir"})
	}

	if _, err := c.api.Delete(ids); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	c.logger.Info(fmt.Sprintf("API        delete   %d items", len(ids)))
	if c.stats != nil {
		c.stats.Incr("api")
	}

	for _, r := range rslv {
		c.cache.RemoveEntry(r.parentCID, r.name)
		c.cache.DeletePathPrefix(r.path)
		// If this was a directory, also clear its children from the cache.
		if r.isDir && r.nodeID != "" {
			c.cache.InvalidateDir(r.nodeID)
		}
		c.logger.Debug("WRITE-THRU remove_entry", "cid", r.parentCID, "name", r.name)
	}
	return nil
}

// DeleteByIDs deletes files by their file IDs. optionally refreshing dirs
// after the deletion.
func (c *Client) DeleteByIDs(fids []string, refreshDirs []string) error {
	if len(fids) == 0 {
		return nil
	}
	c.logger.Info(fmt.Sprintf("API        delete   %d items (by id)", len(fids)))
	if c.stats != nil {
		c.stats.Incr("api")
	}
	if _, err := c.api.Delete(fids); err != nil {
		return err
	}
	if len(refreshDirs) > 0 {
		return c.RefreshPaths(refreshDirs)
	}
	return nil
}

// Move moves srcPaths to destPath.
func (c *Client) Move(srcPaths []string, destPath string) error {
	destPath = normalize(destPath)
	destCID, err := c.ResolvePath(destPath)
	if err != nil {
		return err
	}

	type source struct {
		path      string
		parentCID string
		name      string
		entry     Entry
	}

	fids := make([]string, 0, len(srcPaths))
	sources := make([]source, 0, len(srcPaths))

	for _, sp := range srcPaths {
		sp = normalize(sp)
		lastSlash := strings.LastIndex(sp, "/")
		parentPath := sp[:lastSlash]
		if parentPath == "" {
			parentPath = "/"
		}
		name := sp[lastSlash+1:]

		parentCID, err := c.ResolvePath(parentPath)
		if err != nil {
			return err
		}

		e := c.cache.FindEntry(parentCID, name)
		if e == nil {
			if _, err := c.listDirByCID(parentCID, parentPath); err != nil {
				return err
			}
			e = c.cache.FindEntry(parentCID, name)
		}
		if e == nil {
			return fmt.Errorf("not found: %s", sp)
		}

		fids = append(fids, e.NodeID)
		sources = append(sources, source{sp, parentCID, name, *e})
	}

	if _, err := c.api.Move(fids, destCID); err != nil {
		return fmt.Errorf("move: %w", err)
	}
	c.logger.Info(fmt.Sprintf("API        move     %d items", len(fids)), "dest_cid", destCID)
	if c.stats != nil {
		c.stats.Incr("api")
	}

	for _, src := range sources {
		c.cache.MoveEntry(src.parentCID, src.entry.NodeID, destCID)
		c.cache.DeletePathPrefix(src.path)
		if src.entry.Type == "dir" {
			newPath := strings.TrimRight(destPath, "/") + "/" + src.name
			c.cache.SetPath(newPath, src.entry.NodeID)
		}
		c.logger.Debug("WRITE-THRU move_entry", "src", src.path, "dest_cid", destCID)
	}
	return nil
}

// BatchRenameItem pairs a cloud path with its desired new name.
type BatchRenameItem struct {
	Path    string
	NewName string
}

// BatchRename renames multiple files in a single API call.
func (c *Client) BatchRename(renames []BatchRenameItem) error {
	apiRenames := make(map[string]string, len(renames))
	type info struct {
		parentCID string
		nodeID    string
		newName   string
	}
	infos := make([]info, 0, len(renames))

	for _, r := range renames {
		path := normalize(r.Path)
		lastSlash := strings.LastIndex(path, "/")
		parentPath := path[:lastSlash]
		if parentPath == "" {
			parentPath = "/"
		}
		oldName := path[lastSlash+1:]

		parentCID, err := c.ResolvePath(parentPath)
		if err != nil {
			return err
		}

		e := c.cache.FindEntry(parentCID, oldName)
		if e == nil {
			if _, err := c.listDirByCID(parentCID, parentPath); err != nil {
				return err
			}
			e = c.cache.FindEntry(parentCID, oldName)
		}
		if e == nil {
			return fmt.Errorf("not found: %s", path)
		}

		apiRenames[e.NodeID] = r.NewName
		infos = append(infos, info{parentCID, e.NodeID, r.NewName})
	}

	if _, err := c.api.BatchRename(apiRenames); err != nil {
		return fmt.Errorf("batch_rename: %w", err)
	}
	c.logger.Info(fmt.Sprintf("API        batch_rename %d items", len(apiRenames)))
	if c.stats != nil {
		c.stats.Incr("api")
	}

	for _, inf := range infos {
		c.cache.RenameEntry(inf.parentCID, inf.nodeID, inf.newName)
		c.logger.Debug("WRITE-THRU rename_entry", "cid", inf.parentCID, "node_id", inf.nodeID, "new_name", inf.newName)
	}
	return nil
}

// Upload uploads a local file to remoteDir. If filename is empty the local
// file's base name is used. Returns API result data or nil.
func (c *Client) Upload(localPath, remoteDir, filename string) (map[string]any, error) {
	remoteDir = normalize(remoteDir)
	cid, err := c.ResolvePath(remoteDir)
	if err != nil {
		return nil, err
	}
	if filename == "" {
		filename = filepath.Base(localPath)
	}

	data, err := c.api.UploadFile(localPath, cid, filename)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	c.logger.Info("API        upload   "+filename, "cid", cid)
	if c.stats != nil {
		c.stats.Incr("api")
	}

	if data != nil {
		fid, hasFID := data["fid"]
		if hasFID && fid != nil {
			fi, _ := os.Stat(localPath)
			size := int64(0)
			if fi != nil {
				size = fi.Size()
			}
			if sv, ok := data["file_size"].(float64); ok {
				size = int64(sv)
			}
			pickCode, _ := data["pick_code"].(string)
			if pickCode == "" {
				pickCode, _ = data["pickcode"].(string)
			}
			c.cache.AddEntry(cid, Entry{
				Name:     filename,
				Type:     "file",
				NodeID:   fmt.Sprintf("%v", fid),
				FID:      fmt.Sprintf("%v", fid),
				Size:     size,
				PickCode: pickCode,
			})
			c.logger.Debug("WRITE-THRU add_entry", "cid", cid, "name", filename, "type", "file", "fid", fid)
		} else {
			c.cache.DeleteDirMeta(cid)
			c.logger.Debug("WRITE-THRU delete_dir_meta", "cid", cid)
		}
	} else {
		c.cache.DeleteDirMeta(cid)
		c.logger.Debug("WRITE-THRU delete_dir_meta", "cid", cid)
	}

	return data, nil
}

// RapidResult holds the result of a rapid-upload attempt.
type RapidResult struct {
	Status   int
	PickCode string
}

// RapidUpload attempts an instant (hash-match) upload.
func (c *Client) RapidUpload(localPath, remoteDir string) (*RapidResult, error) {
	remoteDir = normalize(remoteDir)
	cid, err := c.ResolvePath(remoteDir)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("rapid_upload stat: %w", err)
	}
	fileSize := fi.Size()

	h := sha1.New()
	f, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("rapid_upload open: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 1024*1024)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, fmt.Errorf("rapid_upload hash: %w", rerr)
		}
	}
	sha1Str := strings.ToUpper(fmt.Sprintf("%x", h.Sum(nil)))

	f2, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("rapid_upload reopen: %w", err)
	}
	defer f2.Close()

	filename := filepath.Base(localPath)
	result, err := c.api.RapidUpload(cid, filename, fileSize, sha1Str, f2)
	if err != nil {
		return nil, fmt.Errorf("rapid_upload: %w", err)
	}

	status := 0
	if sv, ok := result["status"].(float64); ok {
		status = int(sv)
	} else if sv, ok := result["status"].(int); ok {
		status = sv
	}
	pickCode, _ := result["pickcode"].(string)

	c.logger.Info("API        rapid_upload "+filename, "cid", cid, "status", status)
	if c.stats != nil {
		c.stats.Incr("api")
	}

	if status == 2 {
		c.cache.DeleteDirMeta(cid)
		c.logger.Debug("WRITE-THRU delete_dir_meta", "cid", cid)
	}

	return &RapidResult{Status: status, PickCode: pickCode}, nil
}

// ── CACHE MANAGEMENT ─────────────────────────────────────────────────────────

// Warm pre-warms the cache by recursively listing directories to depth.
// progress is called after each directory is listed (may be nil).
func (c *Client) Warm(path string, depth int, progress func(string, int)) error {
	path = normalize(path)
	cid, err := c.ResolvePath(path)
	if err != nil {
		return err
	}
	return c.warmByCID(cid, path, depth, progress)
}

func (c *Client) warmByCID(cid, path string, depth int, progress func(string, int)) error {
	items, err := c.listDirByCID(cid, path)
	if err != nil {
		return err
	}

	// Cache path_index for all child directories.
	for _, item := range items {
		if item.Type == "dir" {
			childPath := strings.TrimRight(path, "/") + "/" + item.Name
			c.cache.SetPath(childPath, item.NodeID)
		}
	}

	if progress != nil {
		progress(path, len(items))
	}

	if depth > 0 {
		for _, item := range items {
			if item.Type == "dir" {
				childPath := strings.TrimRight(path, "/") + "/" + item.Name
				if err := c.warmByCID(item.NodeID, childPath, depth-1, progress); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// RefreshDir forces a refresh of path's listing regardless of TTL.
func (c *Client) RefreshDir(path string) ([]Entry, error) {
	path = normalize(path)
	cid, err := c.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	rawItems, err := c.api.ListFilesAll(cid)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rawItems))
	for _, item := range rawItems {
		entries = append(entries, normalizeItem(item))
	}
	c.cache.SetDirListing(cid, entries)
	c.logger.Debug("REFRESH list_dir", "path", path, "cid", cid, "items", len(entries))
	return entries, nil
}

// RefreshPaths refreshes multiple directories, deduplicating first.
func (c *Client) RefreshPaths(paths []string) error {
	seen := make(map[string]struct{})
	for _, path := range paths {
		path = normalize(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		if _, err := c.RefreshDir(path); err != nil {
			return err
		}
	}
	return nil
}

// Invalidate removes cache entries for path and everything beneath it.
func (c *Client) Invalidate(path string) error {
	path = normalize(path)
	cid, _, ok := c.cache.GetPath(path)
	c.cache.DeletePathPrefix(path)
	if ok && cid != "" {
		c.cache.InvalidateDir(cid)
	}
	return nil
}

// CacheStatus returns a summary of the current cache state.
func (c *Client) CacheStatus() (*CacheStats, error) {
	s := c.cache.Stats()
	return &s, nil
}

// CacheClear clears all cache metadata (keeps rate-limit state).
func (c *Client) CacheClear() error {
	c.cache.ClearMetadata()
	return nil
}

// ── TREE SNAPSHOT ─────────────────────────────────────────────────────────────

// SaveTree parses the 115 export-tree text and saves entries to the cache.
// videoExts is the set of lowercase extensions (e.g. {".mkv", ".mp4"}).
// Returns the number of entries saved.
func (c *Client) SaveTree(text string, videoExts []string, rootPath string) (int, error) {
	extSet := make(map[string]struct{}, len(videoExts))
	for _, e := range videoExts {
		extSet[strings.ToLower(e)] = struct{}{}
	}
	entries := parseTreeText(text, extSet, ".nfo")
	c.cache.SetTree(entries, rootPath)
	return len(entries), nil
}

// TreeEntries returns tree entries, optionally filtered by category path prefix.
func (c *Client) TreeEntries(category string) ([]TreeEntry, error) {
	return c.cache.GetTreeEntries(category), nil
}

// TreeStats holds counts of tree entries by type.
type TreeStats struct {
	Total  int `json:"total"`
	Videos int `json:"videos"`
	NFOs   int `json:"nfos"`
}

// TreeStats returns count totals for the stored tree snapshot.
func (c *Client) TreeStats() (*TreeStats, error) {
	total, videos, nfos := c.cache.TreeStats()
	return &TreeStats{Total: total, Videos: videos, NFOs: nfos}, nil
}

// parseTreeText parses 115 export-tree format into TreeEntry slice.
// Only video files and NFO files are included; directories are skipped.
//
// Format example (each level is indented with "| "):
//
//	|——根目录
//	| |-影音
//	| | |-AV
//	| | | |-ABP-040
//	| | | | |-ABP-040.mkv
func parseTreeText(text string, videoExts map[string]struct{}, nfoExt string) []TreeEntry {
	var entries []TreeEntry
	var pathStack []string

	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		stripped := strings.TrimRight(line, "\r")
		if !strings.Contains(stripped, "|-") {
			continue
		}

		depth := strings.Count(stripped, "| ")
		parts := strings.SplitN(stripped, "|-", 2)
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		if name == "" {
			continue
		}

		for len(pathStack) >= depth && len(pathStack) > 0 {
			pathStack = pathStack[:len(pathStack)-1]
		}
		pathStack = append(pathStack, name)

		fullPath := strings.Join(pathStack, "/")
		dotIdx := strings.LastIndex(name, ".")
		ext := ""
		if dotIdx != -1 {
			ext = strings.ToLower(name[dotIdx:])
		}
		_, isVideo := videoExts[ext]
		isNFO := ext == nfoExt

		if isVideo || isNFO {
			parent := ""
			if len(pathStack) > 1 {
				parent = strings.Join(pathStack[:len(pathStack)-1], "/")
			}
			entries = append(entries, TreeEntry{
				Path:    fullPath,
				Name:    name,
				Parent:  parent,
				IsVideo: isVideo,
				IsNFO:   isNFO,
			})
		}
	}
	return entries
}

// ── AUTH DELEGATION ───────────────────────────────────────────────────────────

// QRLogin performs an interactive QR-code login and refreshes client cookies.
func (c *Client) QRLogin(app string) error {
	return c.api.QRLogin(app)
}

// QRGetToken fetches a QR login token (phase 1 of two-phase login).
func (c *Client) QRGetToken(app string) (*QRSession, error) {
	return c.api.QRGetToken(app)
}

// QRWaitAndLogin polls for QR scan and finalizes login (phase 2).
func (c *Client) QRWaitAndLogin(sess *QRSession) error {
	return c.api.QRWaitAndLogin(sess)
}

// CheckLogin returns true if the current cookies are valid.
func (c *Client) CheckLogin() bool {
	return c.api.CheckLogin()
}

// RenewCookies attempts a silent cookie refresh and returns true on success.
func (c *Client) RenewCookies() bool {
	return c.api.RenewCookies("tv")
}

// SaveCookies updates the TOML config file at path with the current cookies.
func (c *Client) SaveCookies(path string) error {
	return c.api.SaveCookies(path)
}

// GetCookies returns the current cookie string.
func (c *Client) GetCookies() string {
	return c.api.GetCookies()
}
