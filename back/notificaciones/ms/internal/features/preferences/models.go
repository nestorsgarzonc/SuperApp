package preferences

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	CategorySOS       = "SOS"
	CategorySecurity  = "SECURITY"
	CategoryEmergency = "EMERGENCY"
	CategoryCall      = "CALL"

	CategoryGeneral      = "GENERAL"
	CategoryPackage      = "PACKAGE"
	CategoryAnnouncement = "ANNOUNCEMENT"
	CategoryCommunity    = "COMMUNITY"
	CategoryPayment      = "PAYMENT"
	CategoryAssembly     = "ASSEMBLY"
	CategoryEvent        = "EVENT"
	CategoryMaintenance  = "MAINTENANCE"
	CategoryAccess       = "ACCESS"
	CategoryVisitor      = "VISITOR"
	CategoryAccount      = "ACCOUNT"
)

var criticalCategories = map[string]struct{}{
	CategorySOS:       {},
	CategorySecurity:  {},
	CategoryEmergency: {},
	CategoryCall:      {},
}

var configurableCategories = []string{
	CategoryGeneral,
	CategoryPackage,
	CategoryAnnouncement,
	CategoryCommunity,
	CategoryPayment,
	CategoryAssembly,
	CategoryEvent,
	CategoryMaintenance,
	CategoryAccess,
	CategoryVisitor,
	CategoryAccount,
}

func IsCritical(category string) bool {
	_, ok := criticalCategories[category]
	return ok
}

func IsKnown(category string) bool {
	return IsCritical(category) || slices.Contains(configurableCategories, category)
}

func ConfigurableCategories() []string {
	return slices.Clone(configurableCategories)
}

type Preference struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	ComplexID  *uuid.UUID
	Category   string
	Enabled    bool
	QuietStart *int
	QuietEnd   *int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type PreferenceItem struct {
	Category   string `json:"category"`
	Enabled    bool   `json:"enabled"`
	QuietStart *int   `json:"quiet_start"`
	QuietEnd   *int   `json:"quiet_end"`
}

type GetPreferencesResponse struct {
	Timezone    string           `json:"timezone"`
	Preferences []PreferenceItem `json:"preferences"`
}

type UpdatePreferencesRequest struct {
	Timezone    *string          `json:"timezone,omitempty"`
	Preferences []PreferenceItem `json:"preferences"`
}
