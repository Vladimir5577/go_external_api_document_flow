package middleware

import "testing"

// Иерархия ролей — единственная защита этих маршрутов. Если она разъедется
// с security.yaml монолита, админы молча потеряют доступ, а лишние люди получат.
func TestHasRoleFollowsSymfonyHierarchy(t *testing.T) {
	cases := []struct {
		name     string
		roles    []string
		required []string
		want     bool
	}{
		{"профильная роль", []string{"ROLE_HR", "ROLE_USER"}, []string{"ROLE_HR"}, true},
		{"админ наследует HR", []string{"ROLE_ADMIN"}, []string{"ROLE_HR"}, true},
		{"аналитик наследует обращения", []string{"ROLE_ANALYTIC"}, []string{"ROLE_CITIZEN_APPEAL"}, true},
		{"обычный пользователь", []string{"ROLE_USER"}, []string{"ROLE_HR"}, false},
		{"чужая профильная роль", []string{"ROLE_CITIZEN_APPEAL"}, []string{"ROLE_HR"}, false},
		{"ролей нет вовсе", nil, []string{"ROLE_HR"}, false},
	}

	for _, tc := range cases {
		if got := HasRole(tc.roles, tc.required...); got != tc.want {
			t.Errorf("%s: HasRole(%v, %v) = %v, ожидалось %v", tc.name, tc.roles, tc.required, got, tc.want)
		}
	}
}
