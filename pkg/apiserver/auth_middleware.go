package apiserver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/psviderski/uncloud-dns/pkg/backend"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

type ContextKey string

const DomainID ContextKey = "domainID"

func tokenAuthMiddleware(b backend.Backend) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				logrus.Debugf("request URL path: %s", r.URL.Path)
				authorization := r.Header.Get("Authorization")
				token := strings.TrimPrefix(authorization, "Bearer ")
				domainName, ok := mux.Vars(r)["domain"]
				if !ok {
					handleError(w, http.StatusUnauthorized, errors.New("must specify domain"))
				}

				domain, err := b.GetDomain(domainName)
				if err != nil {
					logrus.Errorf("failed to get domain from DB for %v, err: %v", domainName, err)
					writeErrorResponse(w, http.StatusInternalServerError, "Failed to perform authentication", nil)
					return
				}

				//domain doesn't exist
				if domain.ID == 0 {
					// the extra bit of body is to tell acorn that the domain and token it has is not valid, so it should
					// probably be deleted
					writeErrorResponse(
						w, http.StatusUnauthorized, "Authentication failed", map[string]bool{
							"noDomain": true,
						},
					)
					return
				}

				if domain.TokenHash == "" {
					logrus.Errorf("domain %v is missing token hash", domainName)
					writeErrorResponse(w, http.StatusInternalServerError, "Failed to perform authentication", nil)
					return
				}

				if err := bcrypt.CompareHashAndPassword([]byte(domain.TokenHash), []byte(token)); err != nil {
					writeErrorResponse(w, http.StatusUnauthorized, "Authentication failed", nil)
					return
				}

				ctx := context.WithValue(r.Context(), DomainID, domain.ID)
				next.ServeHTTP(w, r.WithContext(ctx))
			},
		)
	}
}

func domainIDFromContext(ctx context.Context) uint {
	domainID, _ := ctx.Value(DomainID).(uint)
	return domainID
}
