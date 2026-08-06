package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// usageSummary aggregates the four upstream usage endpoints into one response.
type usageSummary struct {
	Whoami     json.RawMessage `json:"whoami,omitempty"`
	Credits    json.RawMessage `json:"credits,omitempty"`
	Sub        json.RawMessage `json:"subscription,omitempty"`
	Summary    json.RawMessage `json:"summary,omitempty"`
	Errors     []string        `json:"errors,omitempty"`
	PlanLabel  string          `json:"planLabel,omitempty"`
	PlanID     string          `json:"planId,omitempty"`
	CostUSD    float64         `json:"costUSD"`
	CreditsAvl float64         `json:"creditsAvailable"`
	CreditsTot float64         `json:"creditsTotal"`
	Percent    float64         `json:"percentUsed"`
	DaysLeft   int             `json:"daysLeft"`
	UsageURL   string          `json:"usageUrl,omitempty"`
	// List of recent usage records (from /internal/usage)
	Items       json.RawMessage `json:"items,omitempty"`
	ListCursor  string          `json:"listCursor,omitempty"`
	TotalTokens int64           `json:"totalTokens"`
	TotalRuns   int64           `json:"totalRuns"`
	TokensIn    int64           `json:"tokensIn"`
	TokensOut   int64           `json:"tokensOut"`
}

// handleUsage queries CommandCode's usage endpoints (same calls the CLI /usage
// command makes) and returns a normalized summary. Requires a valid key.
func (p *Proxy) HandleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		p.writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error")
		return
	}

	apiKey := r.Header.Get("Authorization")
	if apiKey == "" {
		apiKey = p.APIKey
	}
	apiKey = trimBearer(apiKey)
	if apiKey == "" {
		p.writeOpenAIError(w, http.StatusUnauthorized, "API key required. Set Authorization header.", "authentication_error")
		return
	}

	get := func(endpoint string, params map[string]string) ([]byte, string, error) {
		u := p.BaseURL + endpoint
		if len(params) > 0 {
			q := url.Values{}
			for k, v := range params {
				q.Set(k, v)
			}
			u += "?" + q.Encode()
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.Client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("%s returned %d: %s", endpoint, resp.StatusCode, truncateLog(string(body)))
		}
		return body, "", nil
	}

	var sum usageSummary
	// 1. whoami -> org id
	whoamiRaw, _, err := get("/alpha/whoami", nil)
	if err != nil {
		sum.Errors = append(sum.Errors, err.Error())
		sum.UsageURL = "https://commandcode.ai/usage"
		p.writeJSON(w, sum)
		return
	}
	sum.Whoami = whoamiRaw
	var whoami struct {
		Org struct {
			ID    string `json:"id"`
			Login string `json:"login"`
		} `json:"org"`
		User struct {
			UserName string `json:"userName"`
		} `json:"user"`
	}
	_ = json.Unmarshal(whoamiRaw, &whoami)
	orgID := whoami.Org.ID
	if orgID == "" {
		sum.Errors = append(sum.Errors, "no org id in whoami")
		p.writeJSON(w, sum)
		return
	}

	// 2. credits
	creditsRaw, _, err := get("/alpha/billing/credits", map[string]string{"orgId": orgID})
	if err == nil {
		sum.Credits = creditsRaw
	}
	// 3. subscription
	subRaw, _, err := get("/alpha/billing/subscriptions", map[string]string{"orgId": orgID})
	if err == nil {
		sum.Sub = subRaw
	}

	// 4. usage summary since period start
	var sub struct {
		Data struct {
			PlanID             string `json:"planId"`
			Status             string `json:"status"`
			CurrentPeriodStart string `json:"currentPeriodStart"`
			CurrentPeriodEnd   string `json:"currentPeriodEnd"`
		} `json:"data"`
	}
	if sum.Sub != nil {
		_ = json.Unmarshal(sum.Sub, &sub)
	}
	var summaryRaw []byte
	if sub.Data.CurrentPeriodStart != "" {
		summaryRaw, _, err = get("/alpha/usage/summary", map[string]string{"orgId": orgID, "since": sub.Data.CurrentPeriodStart})
		if err == nil {
			sum.Summary = summaryRaw
		}
	}
	// aggregate credits
	var credits struct {
		Credits struct {
			MonthlyCredits   float64 `json:"monthlyCredits"`
			PurchasedCredits float64 `json:"purchasedCredits"`
			FreeCredits      float64 `json:"freeCredits"`
		} `json:"credits"`
	}
	if sum.Credits != nil {
		_ = json.Unmarshal(sum.Credits, &credits)
	}
	// total cost
	var summary struct {
		TotalCost float64 `json:"totalCost"`
	}
	if sum.Summary != nil {
		_ = json.Unmarshal(sum.Summary, &summary)
	}
	sum.PlanID = sub.Data.PlanID
	sum.PlanLabel = planLabel(sub.Data.PlanID)
	sum.CreditsAvl = credits.Credits.MonthlyCredits + credits.Credits.PurchasedCredits + credits.Credits.FreeCredits
	sum.CreditsTot = sum.CreditsAvl + summary.TotalCost
	if sum.CreditsTot > 0 {
		sum.Percent = summary.TotalCost / sum.CreditsTot * 100
		if sum.Percent > 100 {
			sum.Percent = 100
		}
	}
	if t, err := time.Parse(time.RFC3339, sub.Data.CurrentPeriodEnd); err == nil {
		sum.DaysLeft = int(time.Until(t).Hours()/24) + 1
		if sum.DaysLeft < 0 {
			sum.DaysLeft = 0
		}
	}
	sum.UsageURL = "https://commandcode.ai/usage"

	// 5. recent usage records list (/internal/usage) — same call the official
	// Usage page makes. Returns items with per-request tokens/cost/model/status.
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "10"
	}
	listParams := map[string]string{"limit": limit, "orgId": orgID}
	if c := r.URL.Query().Get("cursor"); c != "" {
		listParams["cursor"] = c
	}
	if listRaw, _, err := get("/internal/usage", listParams); err == nil {
		var list struct {
			Items  json.RawMessage `json:"items"`
			Cursor string          `json:"cursor"`
			Total  struct {
				TotalTokens int64 `json:"totalTokens"`
				TotalRuns   int64 `json:"totalRuns"`
				TokensIn    int64 `json:"tokensIn"`
				TokensOut   int64 `json:"tokensOut"`
			} `json:"total"`
		}
		if uerr := json.Unmarshal(listRaw, &list); uerr == nil {
			sum.Items = list.Items
			sum.ListCursor = list.Cursor
			sum.TotalTokens = list.Total.TotalTokens
			sum.TotalRuns = list.Total.TotalRuns
			sum.TokensIn = list.Total.TokensIn
			sum.TokensOut = list.Total.TokensOut
		}
	}

	p.writeJSON(w, sum)
}

func (p *Proxy) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func trimBearer(key string) string {
	key = http.CanonicalHeaderKey(key)
	if len(key) > 6 && key[:6] == "Bearer" {
		key = key[7:]
	}
	return key
}

// planLabel maps a plan id to a human-readable label. Unknown ids pass through.
func planLabel(planID string) string {
	labels := map[string]string{
		"go":       "Go",
		"goat":     "GOAT",
		"pro":      "Pro",
		"max_10x":  "Max 10x",
		"max_20x":  "Max 20x",
		"team_pro": "Team Pro",
	}
	if l, ok := labels[planID]; ok {
		return l
	}
	if planID == "" {
		return "Unknown plan"
	}
	return planID
}
