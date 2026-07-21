package collector

import (
	"context"

	"github.com/salarrbl/cexss/pkg/logger"
)

type WaybackCollector struct{}

func NewWaybackCollector() *WaybackCollector {
	return &WaybackCollector{}
}

func (w *WaybackCollector) Fetch(ctx context.Context, target string, out chan<- CollectedURL) error {
	logger.Warning("waybackurls has been replaced by gau. Use GAUCollector instead.")
	return nil
}
