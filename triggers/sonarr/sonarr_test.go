package sonarr

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/cloudbox/autoscan"
)

// scansEqual compares two slices of scans for equality, ignoring order
func scansEqual(expected, actual []autoscan.Scan) bool {
	if len(expected) != len(actual) {
		return false
	}

	// Sort both slices for comparison
	sortScans := func(scans []autoscan.Scan) []autoscan.Scan {
		sorted := make([]autoscan.Scan, len(scans))
		copy(sorted, scans)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].Folder != sorted[j].Folder {
				return sorted[i].Folder < sorted[j].Folder
			}
			if sorted[i].RelativePath != sorted[j].RelativePath {
				return sorted[i].RelativePath < sorted[j].RelativePath
			}
			return sorted[i].Priority < sorted[j].Priority
		})
		return sorted
	}

	expectedSorted := sortScans(expected)
	actualSorted := sortScans(actual)

	return reflect.DeepEqual(expectedSorted, actualSorted)
}

func TestHandler(t *testing.T) {
	type Given struct {
		Config  Config
		Fixture string
	}

	type Expected struct {
		Scans      []autoscan.Scan
		StatusCode int
	}

	type Test struct {
		Name     string
		Given    Given
		Expected Expected
	}

	standardConfig := Config{
		Name:     "sonarr",
		Priority: 5,
		Rewrite: []autoscan.Rewrite{{
			From: "/TV/*",
			To:   "/mnt/unionfs/Media/TV/$1",
		}},
	}

	currentTime := time.Now()
	now = func() time.Time {
		return currentTime
	}

	testCases := []Test{
		{
			"Scan has all the correct fields on Download event",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/westworld.json",
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:       "/mnt/unionfs/Media/TV/Westworld/Season 1",
						RelativePath: "Westworld.S01E01.mkv",
						Priority:     5,
						Time:         currentTime,
					},
				},
			},
		},
		{
			"Scan on EpisodeFileDelete",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/episode_delete.json",
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:       "/mnt/unionfs/Media/TV/Westworld/Season 2",
						RelativePath: "Westworld.S02E01.mkv",
						Priority:     5,
						Time:         currentTime,
					},
				},
			},
		},
		{
			"Picks up the Rename event without duplicates",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/rename.json",
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:       "/mnt/unionfs/Media/TV/Westworld/Season 1",
						RelativePath: "Westworld.S01E01.mkv",
						Priority:     5,
						Time:         currentTime,
					},
					{
						Folder:       "/mnt/unionfs/Media/TV/Westworld [imdb:tt0475784]/Season 1",
						RelativePath: "Westworld.S01E01.mkv",
						Priority:     5,
						Time:         currentTime,
					},
					{
						Folder:       "/mnt/unionfs/Media/TV/Westworld/Season 2",
						RelativePath: "Westworld.S01E02.mkv",
						Priority:     5,
						Time:         currentTime,
					},
					{
						Folder:       "/mnt/unionfs/Media/TV/Westworld [imdb:tt0475784]/Season 2",
						RelativePath: "Westworld.S02E01.mkv",
						Priority:     5,
						Time:         currentTime,
					},
				},
			},
		},
		{
			"Scans show folder on SeriesDelete event",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/series_delete.json",
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:   "/mnt/unionfs/Media/TV/Westworld",
						Priority: 5,
						Time:     currentTime,
					},
				},
			},
		},
		{
			"Returns bad request on invalid JSON",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/invalid.json",
			},
			Expected{
				StatusCode: 400,
			},
		},
		{
			"Returns 200 on Test event without emitting a scan",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/test.json",
			},
			Expected{
				StatusCode: 200,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			callback := func(scans ...autoscan.Scan) error {
				if !scansEqual(tc.Expected.Scans, scans) {
					t.Log("Actual scans:")
					for _, scan := range scans {
						t.Logf("  %+v", scan)
					}
					t.Log("Expected scans:")
					for _, scan := range tc.Expected.Scans {
						t.Logf("  %+v", scan)
					}
					t.Errorf("Scans are not equal")
					return errors.New("scans are not equal")
				}

				return nil
			}

			trigger, err := New(tc.Given.Config)
			if err != nil {
				t.Fatalf("Could not create Sonarr Trigger: %v", err)
			}

			server := httptest.NewServer(trigger(callback))
			defer server.Close()

			request, err := os.Open(tc.Given.Fixture)
			if err != nil {
				t.Fatalf("Could not open the fixture: %s", tc.Given.Fixture)
			}

			res, err := http.Post(server.URL, "application/json", request)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			defer res.Body.Close()
			if res.StatusCode != tc.Expected.StatusCode {
				t.Errorf("Status codes do not match: %d vs %d", res.StatusCode, tc.Expected.StatusCode)
			}
		})
	}
}
