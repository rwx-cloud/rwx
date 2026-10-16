package api

import (
	"net/http"
	"net/url"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type BillingUsageResult struct {
	OrganizationSlug         string        `json:"organization_slug"`
	Period                   BillingPeriod `json:"period"`
	Currency                 string        `json:"currency"`
	UsageCostInDollars       string        `json:"usage_cost_in_dollars"`
	AvailableCreditInDollars string        `json:"available_credit_in_dollars"`
	Summary                  []BillingLine `json:"summary"`
	Days                     []BillingDay  `json:"days"`
}

type BillingPeriod struct {
	Month            string  `json:"month"`
	StartsOn         string  `json:"starts_on"`
	EndsOn           string  `json:"ends_on"`
	FinalizedThrough *string `json:"finalized_through"`
}

type BillingUnit struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type BillingLine struct {
	SKU                string      `json:"sku"`
	Label              string      `json:"label"`
	Qualifiers         []string    `json:"qualifiers"`
	Unit               BillingUnit `json:"unit"`
	Quantity           string      `json:"quantity"`
	UnitPriceInDollars *string     `json:"unit_price_in_dollars"`
	Included           bool        `json:"included"`
	CostInDollars      *string     `json:"cost_in_dollars"`
}

type BillingDay struct {
	Date          string        `json:"date"`
	CostInDollars *string       `json:"cost_in_dollars"`
	Lines         []BillingLine `json:"lines"`
}

func (c Client) BillingUsage(month string) (*BillingUsageResult, error) {
	endpoint := "/api/billing/usage"
	if month != "" {
		endpoint += "?" + url.Values{"month": {month}}.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()
	result := &BillingUsageResult{}
	if err := decodeResponseJSON(resp, result); err != nil {
		return nil, err
	}
	return result, nil
}
