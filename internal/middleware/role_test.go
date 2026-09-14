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

		// Голосовая почта вынесена в отдельную роль. Раскрытие тут в один уровень,
		// поэтому цепочку админ → обращения → голосовая почта карта должна
		// покрывать напрямую, иначе админ молча потеряет доступ.
		{"голосовая почта своей ролью", []string{"ROLE_CITIZEN_APPEAL_VOICEMAIL"}, []string{"ROLE_CITIZEN_APPEAL_VOICEMAIL"}, true},
		{"обращения наследуют голосовую почту", []string{"ROLE_CITIZEN_APPEAL"}, []string{"ROLE_CITIZEN_APPEAL_VOICEMAIL"}, true},
		{"аналитик наследует голосовую почту", []string{"ROLE_ANALYTIC"}, []string{"ROLE_CITIZEN_APPEAL_VOICEMAIL"}, true},
		{"админ наследует голосовую почту", []string{"ROLE_ADMIN"}, []string{"ROLE_CITIZEN_APPEAL_VOICEMAIL"}, true},
		// А обратной дороги нет: голосовая почта не даёт доступа к обращениям.
		{"голосовая почта не даёт обращений", []string{"ROLE_CITIZEN_APPEAL_VOICEMAIL"}, []string{"ROLE_CITIZEN_APPEAL"}, false},
	}

	for _, tc := range cases {
		if got := HasRole(tc.roles, tc.required...); got != tc.want {
			t.Errorf("%s: HasRole(%v, %v) = %v, ожидалось %v", tc.name, tc.roles, tc.required, got, tc.want)
		}
	}
}
