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

// CleanedPathEqual reports whether scanFolder and libraryPath refer to the same path after cleaning.
func CleanedPathEqual(scanFolder, libraryPath string) bool {
	return path.Clean(scanFolder) == path.Clean(libraryPath)
}
