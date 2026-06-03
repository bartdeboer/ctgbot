package simplerbac

type Role string

const (
	RoleRoot     Role = "root"
	RoleAgent    Role = "agent"
	RoleUser     Role = "user"
	RoleElevated Role = "elevated"
	// RoleController identifies a trusted remote ctgbot controller. It is
	// deliberately distinct from RoleRoot so command surfaces can opt into remote
	// control without granting every local-root capability by default.
	RoleController Role = "controller"
)

type Rule struct {
	AnyRole []Role
}

func Any(roles ...Role) Rule {
	out := make([]Role, 0, len(roles))
	for _, role := range roles {
		if role == "" {
			continue
		}
		out = append(out, role)
	}
	return Rule{AnyRole: out}
}

func Public() Rule { return Rule{} }
