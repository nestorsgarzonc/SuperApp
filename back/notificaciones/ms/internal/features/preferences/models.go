package preferences

import "slices"

const (
	CategorySOS       = "SOS"
	CategorySecurity  = "SECURITY"
	CategoryEmergency = "EMERGENCY"
	CategoryAccess    = "ACCESS"
	CategoryVisitor   = "VISITOR"
	CategoryPackage   = "PACKAGE"

	CategoryCall = "CALL"

	CategoryAnnouncement = "ANNOUNCEMENT"
	CategoryCommunity    = "COMMUNITY"
	CategoryEvent        = "EVENT"
	CategoryMaintenance  = "MAINTENANCE"

	CategoryReservation = "RESERVATION"
	CategoryAssembly    = "ASSEMBLY"
	CategoryVote        = "VOTE"

	CategoryAccount = "ACCOUNT"
	CategoryPayment = "PAYMENT"
	CategoryGeneral = "GENERAL"
)

const (
	GroupAccessControl = "ACCESS_CONTROL"
	GroupIntercom      = "INTERCOM"
	GroupCommunity     = "COMMUNITY"
	GroupReservations  = "RESERVATIONS"
	GroupAssemblies    = "ASSEMBLIES"
	GroupVotes         = "VOTES"
	GroupSystem        = "SYSTEM"
)

var categoryGroup = map[string]string{
	CategorySOS:          GroupAccessControl,
	CategorySecurity:     GroupAccessControl,
	CategoryEmergency:    GroupAccessControl,
	CategoryAccess:       GroupAccessControl,
	CategoryVisitor:      GroupAccessControl,
	CategoryPackage:      GroupAccessControl,
	CategoryCall:         GroupIntercom,
	CategoryAnnouncement: GroupCommunity,
	CategoryCommunity:    GroupCommunity,
	CategoryEvent:        GroupCommunity,
	CategoryMaintenance:  GroupCommunity,
	CategoryReservation:  GroupReservations,
	CategoryAssembly:     GroupAssemblies,
	CategoryVote:         GroupVotes,
	CategoryAccount:      GroupSystem,
	CategoryPayment:      GroupSystem,
	CategoryGeneral:      GroupSystem,
}

var criticalCategories = []string{CategorySOS, CategorySecurity, CategoryEmergency, CategoryCall}

type groupSetting struct {
	Group  string
	Locked bool
}

var preferenceGroups = []groupSetting{
	{GroupAccessControl, true},
	{GroupIntercom, true},
	{GroupReservations, false},
	{GroupCommunity, false},
	{GroupAssemblies, false},
	{GroupVotes, false},
}

func IsKnown(category string) bool {
	_, ok := categoryGroup[category]
	return ok
}

func IsCritical(category string) bool {
	return slices.Contains(criticalCategories, category)
}

func GroupOf(category string) string {
	if g, ok := categoryGroup[category]; ok {
		return g
	}
	return GroupSystem
}

func IsGroup(group string) bool {
	for _, g := range categoryGroup {
		if g == group {
			return true
		}
	}
	return false
}

func CategoriesOf(groups []string) []string {
	var out []string
	for c, g := range categoryGroup {
		if slices.Contains(groups, g) {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out
}

func isMutable(group string) bool {
	for _, s := range preferenceGroups {
		if s.Group == group {
			return !s.Locked
		}
	}
	return false
}

type PreferenceItem struct {
	Group   string `json:"group"`
	Enabled bool   `json:"enabled"`
	Locked  bool   `json:"locked"`
}

type GetPreferencesResponse struct {
	Preferences []PreferenceItem `json:"preferences"`
}

type PreferenceUpdate struct {
	Group   string `json:"group"`
	Enabled *bool  `json:"enabled"`
}

type UpdatePreferencesRequest struct {
	Preferences []PreferenceUpdate `json:"preferences"`
}
