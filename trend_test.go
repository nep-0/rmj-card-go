package main

import (
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/nep-0/rmj"
)

func TestTrendCentered(t *testing.T) {
	for _, grade := range []int{0, 11} {
		player := rmj.PlayerStats{Grade: grade, PromotionPositionSum: 75, PromotionAveragePlace: 2.5, RecentPositions: []int{3, 2, 1, 4, 2}}
		var chart strings.Builder
		renderTrend(&chart, player)
		decoder := xml.NewDecoder(strings.NewReader("<svg>" + chart.String() + "</svg>"))
		lines := 0
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "line" {
				continue
			}
			attrs := map[string]string{}
			for _, attr := range start.Attr {
				attrs[attr.Name.Local] = attr.Value
			}
			left, err := strconv.ParseFloat(attrs["x1"], 64)
			if err != nil {
				t.Fatal(err)
			}
			right, err := strconv.ParseFloat(attrs["x2"], 64)
			if err != nil {
				t.Fatal(err)
			}
			if left+right != cardWidth {
				t.Errorf("grade %d plot midpoint = %g, want %d", grade, (left+right)/2, cardWidth/2)
			}
			lines++
		}
		if lines != 3 {
			t.Errorf("grade %d grid lines = %d, want 3", grade, lines)
		}
	}
}
