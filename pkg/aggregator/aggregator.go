package aggregator

import (
	"net/url"

	"github.com/usefathom/fathom/pkg/datastore"
	"github.com/usefathom/fathom/pkg/models"

	log "github.com/sirupsen/logrus"
)

type aggregator struct {
	database datastore.Datastore
}

// New returns a new aggregator instance with the database dependency injected.
func New(db datastore.Datastore) *aggregator {
	return &aggregator{
		database: db,
	}
}

// Run processes the pageviews which are ready to be processed and adds them to daily aggregation
func (agg *aggregator) Run() int {
	// Get unprocessed pageviews
	pageviews, err := agg.database.GetProcessablePageviews()
	if err != nil && err != datastore.ErrNoResults {
		log.Error(err)
		return 0
	}

	//  Do we have anything to process?
	n := len(pageviews)
	if n == 0 {
		return 0
	}

	results := agg.Process(pageviews)

	// update stats
	for _, site := range results.Sites {
		err = agg.database.UpdateSiteStats(site)
		if err != nil {
			log.Error(err)
		}
	}

	for _, pageStats := range results.Pages {
		err = agg.database.UpdatePageStats(pageStats)
		if err != nil {
			log.Error(err)
		}
	}

	for _, referrerStats := range results.Referrers {
		err = agg.database.UpdateReferrerStats(referrerStats)
		if err != nil {
			log.Error(err)
		}
	}

	// finally, remove pageviews that we just processed
	err = agg.database.DeletePageviews(pageviews)
	if err != nil {
		log.Error(err)
	}

	return n
}

// Process processes the given pageviews and returns the (aggregated) results per metric per day
func (agg *aggregator) Process(pageviews []*models.Pageview) *results {
	log.Debugf("processing %d pageviews", len(pageviews))
	results := newResults()

	for _, p := range pageviews {
		site, err := agg.getSiteStats(results, p.Timestamp)
		if err != nil {
			log.Error(err)
			continue
		}
		site.HandlePageview(p)

		pageStats, err := agg.getPageStats(results, p.Timestamp, p.Hostname, p.Pathname)
		if err != nil {
			log.Error(err)
			continue
		}
		pageStats.HandlePageview(p)

		// referrer stats
		if p.Referrer != "" {

			hostname, pathname, err := parseUrlParts(p.Referrer)
			if err != nil {
				log.Error(err)
				continue
			}

			referrerStats, err := agg.getReferrerStats(results, p.Timestamp, hostname, pathname)
			if err != nil {
				log.Error(err)
				continue
			}
			referrerStats.HandlePageview(p)
		}

	}

	return results
}

func parseUrlParts(s string) (string, string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", "", err
	}

	return u.Scheme + "://" + u.Host, u.Path, nil
}
