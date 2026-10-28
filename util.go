package autoscan

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// JoinURL joins a base URL with additional path segments, handling leading/trailing slashes correctly.
func JoinURL(base string, paths ...string) string {
	// credits: https://stackoverflow.com/a/57220413
	p := path.Join(paths...)
	return fmt.Sprintf("%s/%s", strings.TrimRight(base, "/"), strings.TrimLeft(p, "/"))
}

// DSN creates a data source name for use with sql.Open.
func DSN(dbPath string, q url.Values) string {
	u := url.URL{
		Scheme:   "file",
		Path:     dbPath,
		RawQuery: q.Encode(),
	}

	return u.String()
}

// PathWithinRoot reports whether folder is root or lies below it. Both paths
// are compared after path.Clean, so a trailing slash on either side does not
// matter, and /mnt/media/movies does not contain /mnt/media/movies-int. The
// root "/" contains every absolute path.
func PathWithinRoot(folder, root string) bool {
	f := path.Clean(folder)
	r := path.Clean(root)
	if f == r {
		return true
	}
	if r == "/" {
		return strings.HasPrefix(f, "/")
	}
	return strings.HasPrefix(f, r+"/")
}
