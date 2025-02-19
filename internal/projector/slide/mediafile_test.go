package slide_test

import (
	"context"
	"testing"

	"github.com/OpenSlides/openslides-autoupdate-service/internal/projector"
	"github.com/OpenSlides/openslides-autoupdate-service/internal/projector/datastore"
	"github.com/OpenSlides/openslides-autoupdate-service/internal/projector/slide"
	"github.com/OpenSlides/openslides-autoupdate-service/pkg/dsmock"
	"github.com/stretchr/testify/assert"
)

func TestMediafile(t *testing.T) {
	s := new(projector.SlideStore)
	slide.Mediafile(s)

	mfSlide := s.GetSlider("mediafile")
	assert.NotNilf(t, mfSlide, "Slide with name `mediafile` not found.")

	data := dsmock.YAMLData(`
	mediafile/1/mimetype: application/pdf
    `)

	for _, tt := range []struct {
		name   string
		data   map[string][]byte
		expect string
	}{
		{
			"Starter",
			data,
			`{
				"id": 1,
				"mimetype": "application/pdf"
			}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			shutdownCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			fetch := datastore.NewFetcher(dsmock.NewMockDatastore(shutdownCtx.Done(), tt.data))

			p7on := &projector.Projection{
				ContentObjectID: "mediafile/1",
			}

			bs, err := mfSlide.Slide(context.Background(), fetch, p7on)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.expect, string(bs))
		})
	}
}
