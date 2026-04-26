package simplerbac

type Role string

const (
	RoleRoot     Role = "root"
	RoleAgent    Role = "agent"
	RoleUser     Role = "user"
	RoleElevated Role = "elevated"
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
