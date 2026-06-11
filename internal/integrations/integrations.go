// Package integrations is the connector registry for Lookout. It describes every
// integration (ticketing, MDM, identity, security, notification channels), what
// credentials each will need, and whether it's live or still in development.
//
// Connectors marked "in development" are scaffolded here so the UI and the
// configuration surface exist; the live API wiring lands per-connector once the
// operator supplies credentials.
package integrations

// Status is whether a connector is usable today.
type Status string

const (
	StatusLive          Status = "live"
	StatusInDevelopment Status = "in_development"
)

func (s Status) Label() string {
	if s == StatusLive {
		return "Live"
	}
	return "In development"
}

// Tag is the CSS modifier class for the status badge.
func (s Status) Tag() string {
	if s == StatusLive {
		return "live"
	}
	return "soon"
}

// Integration describes one connector.
type Integration struct {
	ID          string
	Name        string
	Category    string // notifications, ticketing, mdm, identity, security
	Description string
	Status      Status
	Needs       []string // credentials/fields the operator will provide
	EnableHint  string   // how to turn it on today (for live connectors)
}

// Categories in display order.
var Categories = []struct{ Key, Title string }{
	{"security", "Security posture"},
	{"ticketing", "Ticketing"},
	{"mdm", "Device management (MDM)"},
	{"identity", "Identity & directory"},
}

// NotificationCategory is rendered on the Notifications page.
const NotificationCategory = "notifications"

var catalog = []Integration{
	// ── notifications ──
	{ID: "slack", Name: "Slack", Category: "notifications", Status: StatusLive,
		Description: "Post alerts to a Slack channel via an incoming webhook.",
		EnableHint:  "Add the channel's incoming-webhook URL to LOOKOUT_ALERT_WEBHOOKS."},
	{ID: "teams", Name: "Microsoft Teams", Category: "notifications", Status: StatusLive,
		Description: "Post alerts to a Teams channel via an incoming webhook.",
		EnableHint:  "Add the channel's incoming-webhook URL to LOOKOUT_ALERT_WEBHOOKS."},
	{ID: "webhook", Name: "Webhook", Category: "notifications", Status: StatusLive,
		Description: "POST every alert as JSON to any endpoint (PagerDuty, Opsgenie, your own).",
		EnableHint:  "Add the endpoint URL to LOOKOUT_ALERT_WEBHOOKS (comma-separated for many)."},
	{ID: "email", Name: "Email", Category: "notifications", Status: StatusInDevelopment,
		Description: "Email a plain-English summary of what changed and what to do.",
		Needs:       []string{"SMTP host", "SMTP port", "Username", "Password", "From address"}},
	{ID: "sms", Name: "SMS / Text", Category: "notifications", Status: StatusInDevelopment,
		Description: "Text the on-call person on critical alerts.",
		Needs:       []string{"Twilio Account SID", "Auth token", "From number"}},

	// ── security ──
	{ID: "sightline", Name: "Sightline", Category: "security", Status: StatusInDevelopment,
		Description: "Pull live security & compliance posture (NIST / HIPAA / SOC 2) into Lookout.",
		Needs:       []string{"Sightline API base URL", "API key"}},

	// ── ticketing ──
	{ID: "jira", Name: "Jira", Category: "ticketing", Status: StatusInDevelopment,
		Description: "Open a Jira issue automatically when an alert fires.",
		Needs:       []string{"Jira base URL", "Email", "API token", "Project key"}},
	{ID: "servicenow", Name: "ServiceNow", Category: "ticketing", Status: StatusInDevelopment,
		Description: "Create a ServiceNow incident from an alert.",
		Needs:       []string{"Instance URL", "Username", "Password"}},
	{ID: "asana", Name: "Asana", Category: "ticketing", Status: StatusInDevelopment,
		Description: "File an Asana task for the on-call owner.",
		Needs:       []string{"Personal access token", "Workspace ID", "Project ID"}},
	{ID: "trello", Name: "Trello", Category: "ticketing", Status: StatusInDevelopment,
		Description: "Add a Trello card to your ops board.",
		Needs:       []string{"API key", "Token", "Board ID", "List ID"}},

	// ── mdm ──
	{ID: "jamf", Name: "Jamf", Category: "mdm", Status: StatusInDevelopment,
		Description: "Enrich macOS hosts with Jamf inventory and compliance.",
		Needs:       []string{"Jamf Pro URL", "Client ID", "Client secret"}},
	{ID: "intune", Name: "Microsoft Intune", Category: "mdm", Status: StatusInDevelopment,
		Description: "Pull device compliance and configuration from Intune.",
		Needs:       []string{"Tenant ID", "Client ID", "Client secret"}},
	{ID: "kandji", Name: "Kandji", Category: "mdm", Status: StatusInDevelopment,
		Description: "Apple device management signals from Kandji.",
		Needs:       []string{"Subdomain", "API token"}},
	{ID: "jumpcloud", Name: "JumpCloud", Category: "mdm", Status: StatusInDevelopment,
		Description: "Directory + device posture from JumpCloud.",
		Needs:       []string{"API key"}},

	// ── identity ──
	{ID: "activedirectory", Name: "Active Directory", Category: "identity", Status: StatusInDevelopment,
		Description: "Resolve users, groups, and machines from AD over LDAP.",
		Needs:       []string{"LDAP URL", "Bind DN", "Bind password", "Base DN"}},
}

// Catalog returns every integration.
func Catalog() []Integration { return catalog }

// ByCategory returns integrations in one category.
func ByCategory(cat string) []Integration {
	var out []Integration
	for _, in := range catalog {
		if in.Category == cat {
			out = append(out, in)
		}
	}
	return out
}

// ByID looks up one integration.
func ByID(id string) (Integration, bool) {
	for _, in := range catalog {
		if in.ID == id {
			return in, true
		}
	}
	return Integration{}, false
}
