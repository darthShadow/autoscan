package autoscan

import (
	"net/url"
	"testing"
)

func TestDSN(t *testing.T) {
	type Test struct {
		Name  string
		Path  string
		Query url.Values
		Want  string
	}

	testCases := []Test{
		{
			Name: "readonly",
			Path: "library.db",
			Query: url.Values{
				"mode": []string{"ro"},
			},
			Want: "file://library.db?mode=ro",
		},
		{
			Name: "No Query",
			Path: "library.db",
			Want: "file://library.db",
		},
		{
			Name: "Path from root",
			Path: "/mnt/unionfs/library.db",
			Query: url.Values{
				"mode": []string{"rw"},
			},
			Want: "file:///mnt/unionfs/library.db?mode=rw",
		},
		{
			Name: "Query sorted by key",
			Path: "library.db",
			Query: url.Values{
				"cache": []string{"shared"},
				"mode":  []string{"rw"},
			},
			Want: "file://library.db?cache=shared&mode=rw",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			dsn := DSN(tc.Path, tc.Query)
			if dsn != tc.Want {
				t.Log(dsn)
				t.Log(tc.Want)
				t.Error("DSNs do not match")
			}
		})
	}
}

func TestPathWithinRoot(t *testing.T) {
	for _, test := range []struct {
		name, folder, root string
		want               bool
	}{
		{"descendant", "/mnt/media/movies/X (2020)", "/mnt/media/movies", true},
		{"equal", "/mnt/media/movies", "/mnt/media/movies", true},
		{"root-slash", "/mnt/media/movies", "/mnt/media/movies/", true},
		{"folder-slash", "/mnt/media/movies/", "/mnt/media/movies", true},
		{"cleaned", "/mnt/media/movies/./X//", "/mnt/media/movies", true},
		{"movies-int", "/mnt/media/movies-int/X", "/mnt/media/movies", false},
		{"movies-int-slash", "/mnt/media/movies-int/X", "/mnt/media/movies/", false},
		{"ancestor", "/mnt/media", "/mnt/media/movies", false},
		{"traversal", "/mnt/media/movies/../tv/X", "/mnt/media/movies", false},
		{"empty-root", "/mnt/media/movies", "", false},
		{"filesystem-root", "/mnt/media/movies", "/", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := PathWithinRoot(test.folder, test.root); got != test.want {
				t.Errorf("PathWithinRoot(%q, %q) = %v, want %v", test.folder, test.root, got, test.want)
			}
		})
	}
}
