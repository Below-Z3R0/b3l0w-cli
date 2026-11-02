package ytmusic

import (
	"encoding/json"
	"testing"
)

// Respuestas recortadas de innertube. Si YouTube cambia la forma, estos tests
// fallan antes que el panel muestre una lista vacía.
const responsiveFixture = `{
  "contents": {"sectionListRenderer": {"contents": [
    {"musicShelfRenderer": {"title": {"runs": [{"text": "Top songs"}]}, "contents": [
      {"musicResponsiveListItemRenderer": {
        "playlistItemData": {"videoId": "abc123"},
        "flexColumns": [
          {"musicResponsiveListItemFlexColumnRenderer": {"text": {"runs": [{"text": "Get Lucky"}]}}},
          {"musicResponsiveListItemFlexColumnRenderer": {"text": {"runs": [
            {"text": "Daft Punk"}, {"text": " • "}, {"text": "Random Access Memories"}, {"text": " • "}, {"text": "6:09"}
          ]}}}
        ],
        "thumbnail": {"musicThumbnailRenderer": {"thumbnail": {"thumbnails": [
          {"url": "https://example.test/small.jpg", "width": 60},
          {"url": "https://example.test/big.jpg", "width": 226}
        ]}}}
      }}
    ], "continuations": [{"nextContinuationData": {"continuation": "TOKEN-1"}}]}}
  ]}}
}`

const panelFixture = `{
  "contents": {"singleColumnMusicWatchNextResultsRenderer": {"tabbedRenderer": {"watchNextTabbedResultsRenderer": {
    "playlistPanelRenderer": {"contents": [
      {"playlistPanelVideoRenderer": {
        "videoId": "seed01",
        "title": {"runs": [{"text": "Thriller"}]},
        "longBylineText": {"runs": [{"text": "Michael Jackson"}, {"text": " • "}, {"text": "1.2B views"}]},
        "lengthText": {"runs": [{"text": "5:58"}]},
        "thumbnail": {"thumbnails": [{"url": "https://example.test/cover.jpg", "width": 302}]}
      }}
    ], "continuations": [{"nextRadioContinuationData": {"continuation": "RADIO-NEXT"}}]}
  }}}}
}`

const chartsFixture = `{
  "contents": {
    "singleColumnBrowseResultsRenderer": {
      "tabs": [
        {
          "tabRenderer": {
            "content": {
              "sectionListRenderer": {
                "contents": [
                  {
                    "musicCarouselShelfRenderer": {
                      "header": {"musicCarouselShelfBasicHeaderRenderer": {"title": {"runs": [{"text": "Video charts"}]}}},
                      "contents": [
                        {"musicTwoRowItemRenderer": {"title": {"runs": [{"text": "Trending 20 Panama"}]},
                          "navigationEndpoint": {"browseEndpoint": {"browseId": "VLtrending"}}}},
                        {"musicTwoRowItemRenderer": {"title": {"runs": [{"text": "Top 100 Music Videos Panama"}]},
                          "navigationEndpoint": {"browseEndpoint": {"browseId": "VLtop100"}}}}
                      ]
                    }
                  }
                ]
              }
            }
          }
        }
      ]
    }
  }
}`

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("fixture inválido: %v", err)
	}
	return payload
}

func TestSongsFromRenderers(t *testing.T) {
	payload := decode(t, responsiveFixture)
	songs := songsFromRenderers(payload)
	if len(songs) != 1 {
		t.Fatalf("esperaba 1 canción, obtuve %d", len(songs))
	}
	song := songs[0]
	if song.VideoID != "abc123" {
		t.Errorf("videoId = %q", song.VideoID)
	}
	if song.Title != "Get Lucky" {
		t.Errorf("title = %q", song.Title)
	}
	if song.Artist != "Daft Punk" {
		t.Errorf("artist = %q", song.Artist)
	}
	if song.Album != "Random Access Memories" {
		t.Errorf("album = %q", song.Album)
	}
	if song.Duration != "6:09" || song.Seconds != 369 {
		t.Errorf("duración = %q (%d s)", song.Duration, song.Seconds)
	}
	if song.ThumbURL != "https://example.test/big.jpg" {
		t.Errorf("thumb = %q", song.ThumbURL)
	}
}

func TestPlaylistPanelSongsAndContinuation(t *testing.T) {
	payload := decode(t, panelFixture)
	songs := songsFromRenderers(payload)
	if len(songs) != 1 {
		t.Fatalf("esperaba 1 canción, obtuve %d", len(songs))
	}
	if songs[0].Artist != "Michael Jackson" {
		t.Errorf("artist = %q", songs[0].Artist)
	}
	if songs[0].Seconds != 358 {
		t.Errorf("seconds = %d", songs[0].Seconds)
	}
	if token := continuationOf(payload); token != "RADIO-NEXT" {
		t.Errorf("continuation = %q", token)
	}
}

func TestPlaylistCardsPreferTop100(t *testing.T) {
	cards := playlistCards(decode(t, chartsFixture))
	if len(cards) != 2 {
		t.Fatalf("esperaba 2 tarjetas, obtuve %d", len(cards))
	}
	if cards[0].BrowseID != "VLtrending" {
		t.Errorf("orden inesperado: %+v", cards)
	}
	if cards[1].Title != "Top 100 Music Videos Panama" {
		t.Errorf("título = %q", cards[1].Title)
	}
}

func TestFeedFromFailsWithoutSongs(t *testing.T) {
	if _, err := feedFrom("home", map[string]any{"contents": map[string]any{}}); err == nil {
		t.Fatal("un feed sin canciones tiene que ser error, no una lista vacía")
	}
}

func TestDurationToSeconds(t *testing.T) {
	cases := map[string]int{"3:45": 225, "1:02:03": 3723, "0:59": 59, "": 0, "abc": 0, "5": 5}
	for input, want := range cases {
		if got := durationToSeconds(input); got != want {
			t.Errorf("durationToSeconds(%q) = %d, quería %d", input, got, want)
		}
	}
}

func TestSplitMetaIgnoresPlayCounts(t *testing.T) {
	artist, album, duration := splitMeta([]string{"Michael Jackson • 1.2B plays"})
	if artist != "Michael Jackson" {
		t.Errorf("artist = %q", artist)
	}
	if album != "" {
		t.Errorf("album no debería ser el contador de reproducciones: %q", album)
	}
	if duration != "" {
		t.Errorf("duration = %q", duration)
	}
}

func TestSongDedupKeepsOrder(t *testing.T) {
	songs := songDedup([]Song{{VideoID: "a"}, {VideoID: "b"}, {VideoID: "a"}, {VideoID: ""}})
	if len(songs) != 2 || songs[0].VideoID != "a" || songs[1].VideoID != "b" {
		t.Fatalf("dedupe inesperado: %+v", songs)
	}
}
