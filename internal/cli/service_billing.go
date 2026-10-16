package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type BillingUsageConfig struct {
	Month string
	Json  bool
}

type BillingUsageResult struct {
	OrganizationSlug         string
	Period                   BillingPeriod
	Currency                 string
	UsageCostInDollars       string
	AvailableCreditInDollars string
	Summary                  []BillingLine
	Days                     []BillingDay
}

type BillingPeriod struct {
	Month            string
	StartsOn         string
	EndsOn           string
	FinalizedThrough *string
}

type BillingUnit struct {
	Code  string
	Label string
}

type BillingLine struct {
	SKU                string
	Label              string
	Qualifiers         []string
	Unit               BillingUnit
	Quantity           string
	UnitPriceInDollars *string
	Included           bool
	CostInDollars      *string
}

type BillingDay struct {
	Date          string
	CostInDollars *string
	Lines         []BillingLine
}

func (cfg BillingUsageConfig) Validate() error {
	if cfg.Month == "" {
		return nil
	}
	month, err := time.Parse("2006-01", cfg.Month)
	if err != nil || month.Format("2006-01") != cfg.Month || month.Year() == 0 {
		return errors.New("month must be in YYYY-MM format")
	}
	return nil
}

func (s Service) BillingUsage(cfg BillingUsageConfig) (*BillingUsageResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}
	response, err := s.APIClient.BillingUsage(cfg.Month)
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve billing usage")
	}
	result := &BillingUsageResult{
		OrganizationSlug:         response.OrganizationSlug,
		Period:                   BillingPeriod(response.Period),
		Currency:                 response.Currency,
		UsageCostInDollars:       response.UsageCostInDollars,
		AvailableCreditInDollars: response.AvailableCreditInDollars,
		Summary:                  billingLines(response.Summary),
		Days:                     make([]BillingDay, len(response.Days)),
	}
	for i, day := range response.Days {
		result.Days[i] = BillingDay{Date: day.Date, CostInDollars: day.CostInDollars, Lines: billingLines(day.Lines)}
	}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
		return result, nil
	}
	w := tabwriter.NewWriter(s.Stdout, 0, 0, 2, ' ', 0)
	finalized := "None"
	if result.Period.FinalizedThrough != nil {
		finalized = *result.Period.FinalizedThrough
	}
	fmt.Fprintf(w, "Organization: %s\nMonth: %s\nPeriod: %s to %s\nFinalized through: %s\nCurrency: %s\nUsage cost: %s\nAvailable credit: %s\n", result.OrganizationSlug, result.Period.Month, result.Period.StartsOn, result.Period.EndsOn, finalized, result.Currency, result.UsageCostInDollars, result.AvailableCreditInDollars)
	fmt.Fprintln(w, "\nSummary:")
	if len(result.Summary) == 0 {
		fmt.Fprintln(w, "No summarized usage.")
	} else {
		writeBillingLines(w, result.Summary)
	}
	fmt.Fprintln(w, "\nDays:")
	if len(result.Days) == 0 {
		fmt.Fprintln(w, "No daily usage.")
	}
	for _, day := range result.Days {
		fmt.Fprintf(w, "%s\tCost: %s\n", day.Date, billingDecimal(day.CostInDollars))
		if len(day.Lines) > 0 {
			writeBillingLines(w, day.Lines)
		}
	}
	if err := w.Flush(); err != nil {
		return nil, errors.Wrap(err, "unable to write billing usage")
	}
	return result, nil
}

func billingLines(lines []api.BillingLine) []BillingLine {
	result := make([]BillingLine, len(lines))
	for i, line := range lines {
		qualifiers := append([]string{}, line.Qualifiers...)
		result[i] = BillingLine{SKU: line.SKU, Label: line.Label, Qualifiers: qualifiers, Unit: BillingUnit(line.Unit), Quantity: line.Quantity, UnitPriceInDollars: line.UnitPriceInDollars, Included: line.Included, CostInDollars: line.CostInDollars}
	}
	return result
}

func writeBillingLines(w io.Writer, lines []BillingLine) {
	fmt.Fprintln(w, "SKU\tLABEL\tQUALIFIERS\tQUANTITY\tUNIT\tUNIT CODE\tUNIT PRICE\tINCLUDED\tCOST")
	for _, line := range lines {
		price := billingDecimal(line.UnitPriceInDollars)
		if line.Included && line.UnitPriceInDollars == nil {
			price = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%t\t%s\n", line.SKU, line.Label, strings.Join(line.Qualifiers, ", "), line.Quantity, line.Unit.Label, line.Unit.Code, price, line.Included, billingDecimal(line.CostInDollars))
	}
}

func billingDecimal(value *string) string {
	if value == nil {
		return "Pending"
	}
	return *value
}
