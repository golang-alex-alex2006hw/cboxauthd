package handlers

import (
	"net/http"
	"time"

	"github.com/cernbox/cboxauthd/pkg"

	"github.com/dgrijalva/jwt-go"
	"go.uber.org/zap"
)

func CheckAuth(logger *zap.Logger, userBackend pkg.UserBackend, signingKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// try to validate token if set on the cookie
		authCookie, err := r.Cookie("oc_sessionpassphrase")
		if err == nil {
			// validate that the jwt token in the cookie is valid
			_, err = jwt.Parse(authCookie.Value, func(token *jwt.Token) (interface{}, error) {
				return []byte(signingKey), nil
			})
			if err == nil {
				// the token set in the cookie is valid
				w.WriteHeader(http.StatusOK)
				return
			}
			logger.Warn("token in cookie no longer valid")
		}

		// try to get credentials using basic auth
		u, p, ok := r.BasicAuth()
		if !ok {
			logger.Warn("no basic auth provided")
			w.Header().Set("WWW-Authenticate", "Basic Realm='cboxauthd credentials'")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if u == "" || p == "" {
			logger.Warn("empty basic auth credentials")
			w.Header().Set("WWW-Authenticate", "Basic Realm='cboxauthd credentials'")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		err = userBackend.Authenticate(r.Context(), u, p)
		if err != nil {
			if ube, ok := err.(pkg.UserBackendError); ok {
				if ube.Code == pkg.UserBackendErrorNotFound {
					logger.Warn("user not found", zap.String("username", u))
					w.Header().Set("WWW-Authenticate", "Basic Realm='cboxauthd credentials'")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
			}
			logger.Error("authentication failed", zap.Error(err), zap.String("username", u))
			w.Header().Set("WWW-Authenticate", "Basic Realm='cboxauthd credentials'")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// user is authenticated using basic auth so we can cache it in a JWT token
		token := jwt.New(jwt.GetSigningMethod("HS256"))
		claims := token.Claims.(jwt.MapClaims)
		claims["username"] = u
		claims["exp"] = time.Now().Add(time.Second * 3600).UnixNano() // TODO(labkode): expire data in config
		tokenString, _ := token.SignedString([]byte(signingKey))

		// store jwt token in cookie header
		cookie := &http.Cookie{}
		cookie.Name = "oc_sessionpassphrase"
		cookie.Value = tokenString
		http.SetCookie(w, cookie)

		w.WriteHeader(http.StatusOK)
	})
}