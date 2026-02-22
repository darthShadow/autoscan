package autoscan

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestLimitReadCloserTruncates(t *testing.T) {
	overSize := maxResponseBodySize + 1024
	body := strings.Repeat("x", overSize)
	rc := io.NopCloser(bytes.NewBufferString(body))
	limited := LimitReadCloser(rc)

	data, err := io.ReadAll(limited)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if int64(len(data)) != maxResponseBodySize {
		t.Errorf("got %d bytes, want %d", len(data), maxResponseBodySize)
	}

	if err := limited.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func TestRewriter(t *testing.T) {
	type Test struct {
		Name     string
		Rewrites []Rewrite
		Input    string
		Expected string
	}

	testCases := []Test{
		{
			Name:     "One parameter with wildcard",
			Input:    "/mnt/unionfs/Media/Movies/Example Movie/movie.mkv",
			Expected: "/data/Movies/Example Movie/movie.mkv",
			Rewrites: []Rewrite{{
				From: "/mnt/unionfs/Media/",
				To:   "/data/",
			}},
		},
		{
			Name:     "One parameter with glob thingy",
			Input:    "/Media/Movies/test.mkv",
			Expected: "/data/Movies/test.mkv",
			Rewrites: []Rewrite{{
				From: "/Media/(.*)",
				To:   "/data/$1",
			}},
		},
		{
			Name:     "No wildcard",
			Input:    "/Media/whatever",
			Expected: "/whatever",
			Rewrites: []Rewrite{{
				From: "^/Media/",
				To:   "/",
			}},
		},
		{
			Name:     "Unicode (PAS issue #73)",
			Input:    "/media/b33f/saitoh183/private/Videos/FrenchTV/L'échappée/Season 03",
			Expected: "/Videos/FrenchTV/L'échappée/Season 03",
			Rewrites: []Rewrite{{
				From: "/media/b33f/saitoh183/private/",
				To:   "/",
			}},
		},
		{
			Name:     "Returns input when no rules are given",
			Input:    "/mnt/unionfs/test/example.mp4",
			Expected: "/mnt/unionfs/test/example.mp4",
		},
		{
			Name:     "Returns input when rule does not match",
			Input:    "/test/example.mp4",
			Expected: "/test/example.mp4",
			Rewrites: []Rewrite{{
				From: "^/Media/",
				To:   "/mnt/unionfs/Media/",
			}},
		},
		{
			Name:     "Uses second rule if first one does not match",
			Input:    "/test/example.mp4",
			Expected: "/mnt/unionfs/example.mp4",
			Rewrites: []Rewrite{
				{From: "^/Media/", To: "/mnt/unionfs/Media/"},
				{From: "^/test/", To: "/mnt/unionfs/"},
			},
		},
		{
			Name:     "Hotio",
			Input:    "/movies4k/example.mp4",
			Expected: "/mnt/unionfs/movies4k/example.mp4",
			Rewrites: []Rewrite{
				{From: "^/movies/", To: "/mnt/unionfs/movies/"},
				{From: "^/movies4k/", To: "/mnt/unionfs/movies4k/"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			rewriter, err := NewRewriter(tc.Rewrites)
			if err != nil {
				t.Fatal(err)
			}

			result := rewriter(tc.Input)
			if result != tc.Expected {
				t.Errorf("%s does not equal %s", result, tc.Expected)
			}
		})
	}
}
