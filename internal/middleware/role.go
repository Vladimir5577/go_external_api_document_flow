package middleware

import (
	"net/http"
)

// roleHierarchy повторяет role_hierarchy из symfony_documents_flow/config/packages/security.yaml.
// Symfony раскрывает иерархию на стороне проверки, а в JWT-клейме roles лежат «сырые» роли
// пользователя — поэтому раскрывать надо и здесь, иначе ROLE_ADMIN потеряет доступ.
//
// Здесь только те роли, которые дают доступ к маршрутам этого сервиса.
//
// ВАЖНО: карта развёрнута заранее, потому что HasRole раскрывает ровно один
// уровень. В Symfony иерархия транзитивна — там ROLE_ADMIN дотягивается до
// ROLE_CITIZEN_APPEAL_VOICEMAIL через ROLE_CITIZEN_APPEAL. Здесь так не выйдет,
// поэтому наследники родителей выписаны в списки родителей явно.
//
// ponytail: при добавлении новой роли-родителя в security.yaml карту надо руками синхронизировать.
var roleHierarchy = map[string][]string{
	"ROLE_ADMIN":          {"ROLE_ANALYTIC", "ROLE_CITIZEN_APPEAL", "ROLE_CITIZEN_APPEAL_VOICEMAIL", "ROLE_HR", "ROLE_CONTRACT_APPLICATION", "ROLE_USER"},
	"ROLE_ANALYTIC":       {"ROLE_CITIZEN_APPEAL", "ROLE_CITIZEN_APPEAL_VOICEMAIL", "ROLE_HR", "ROLE_CONTRACT_APPLICATION", "ROLE_USER"},
	"ROLE_CITIZEN_APPEAL": {"ROLE_CITIZEN_APPEAL_VOICEMAIL", "ROLE_USER"},
}

// HasRole проверяет наличие любой из требуемых ролей с учётом иерархии.
func HasRole(userRoles []string, required ...string) bool {
	granted := make(map[string]struct{}, len(userRoles)*2)
	for _, role := range userRoles {
		granted[role] = struct{}{}
		for _, inherited := range roleHierarchy[role] {
			granted[inherited] = struct{}{}
		}
	}

	for _, role := range required {
		if _, ok := granted[role]; ok {
			return true
		}
	}

	return false
}

// RequireRole — аналог #[IsGranted] на контроллере Symfony: пускает, если есть хотя бы одна из ролей.
func RequireRole(required ...string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUser(r.Context())
			if !ok {
				unauthorized(w)
				return
			}

			if !HasRole(user.Roles, required...) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"forbidden","code":"forbidden"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
