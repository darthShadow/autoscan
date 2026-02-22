package manual

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/cloudbox/autoscan"
)

func TestHandler(t *testing.T) {
	type Given struct {
		Config Config
		Query  url.Values
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
		Priority: 5,
		Rewrite: []autoscan.Rewrite{
			{
				From: "/Movies/*",
				To:   "/mnt/unionfs/Media/Movies/$1",
			}, {
				From: "/TV/*",
				To:   "/mnt/unionfs/Media/TV/$1",
			},
		},
	}

	currentTime := time.Now()
	now = func() time.Time {
		return currentTime
	}

	testCases := []Test{
		{
			"Returns bad request when no directories are given",
			Given{
				Config: standardConfig,
				Query: url.Values{
					"dir": []string{},
				},
			},
			Expected{
				StatusCode: 400,
			},
		},
		{
			"Returns 200 when given multiple directories",
			Given{
				Config: standardConfig,
				Query: url.Values{
					"dir": []string{
						"/Movies/Interstellar (2014)",
						"/Movies/Parasite (2019)",
					},
				},
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:       "/mnt/unionfs/Media/Movies/Interstellar (2014)",
						RelativePath: "",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
					{
						Folder:       "/mnt/unionfs/Media/Movies/Parasite (2019)",
						RelativePath: "",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
				},
			},
		},
		{
			"Returns 200 when given multiple paths",
			Given{
				Config: standardConfig,
				Query: url.Values{
					"path": []string{
						"/Movies/Interstellar (2014)/Interstellar.mkv",
						"/Movies/Parasite (2019)/Parasite.mkv",
						"/TV/Chernobyl (2019)/Season 1/Chernobyl S01E01.mkv",
					},
				},
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:       "/mnt/unionfs/Media/Movies/Interstellar (2014)",
						RelativePath: "Interstellar.mkv",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
					{
						Folder:       "/mnt/unionfs/Media/Movies/Parasite (2019)",
						RelativePath: "Parasite.mkv",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
					{
						Folder:       "/mnt/unionfs/Media/TV/Chernobyl (2019)/Season 1",
						RelativePath: "Chernobyl S01E01.mkv",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
				},
			},
		},
		{
			"Returns 200 when given multiple paths & directories",
			Given{
				Config: standardConfig,
				Query: url.Values{
					"dir": []string{
						"/Movies/Interstellar (2014)",
						"/TV/Westworld [imdb:tt0475784]/Season 2/Westworld.S02E01.mkv",
					},
					"path": []string{
						"/Movies/Parasite (2019)/Parasite.mkv",
						"/TV/Chernobyl (2019)/Season 1/Chernobyl S01E01.mkv",
					},
				},
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:       "/mnt/unionfs/Media/Movies/Interstellar (2014)",
						RelativePath: "",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
					{
						Folder:       "/mnt/unionfs/Media/TV/Westworld [imdb:tt0475784]/Season 2/Westworld.S02E01.mkv",
						RelativePath: "",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
					{
						Folder:       "/mnt/unionfs/Media/Movies/Parasite (2019)",
						RelativePath: "Parasite.mkv",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
					{
						Folder:       "/mnt/unionfs/Media/TV/Chernobyl (2019)/Season 1",
						RelativePath: "Chernobyl S01E01.mkv",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			callback := func(scans ...autoscan.Scan) error {
				if !reflect.DeepEqual(tc.Expected.Scans, scans) {
					t.Logf("want: %v", tc.Expected.Scans)
					t.Logf("got:  %v", scans)
					t.Error("Scans do not equal")
					return errors.New("Scans do not equal")
				}

				return nil
			}

			trigger, err := New(tc.Given.Config)
			if err != nil {
				t.Fatalf("Could not create Manual Trigger: %v", err)
			}

			server := httptest.NewServer(trigger(callback))
			defer server.Close()

			req, err := http.NewRequest(http.MethodPost, server.URL, http.NoBody)
			if err != nil {
				t.Fatalf("Failed creating request: %v", err)
			}

			req.URL.RawQuery = tc.Given.Query.Encode()

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != tc.Expected.StatusCode {
				t.Errorf("Status codes do not match: %d vs %d", res.StatusCode, tc.Expected.StatusCode)
			}
		})
	}
}
