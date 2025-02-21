package apiserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ghandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/psviderski/uncloud-dns/pkg/backend"
	"github.com/psviderski/uncloud-dns/pkg/version"
	"github.com/sirupsen/logrus"
)

type apiServer struct {
	ctx  context.Context
	log  *logrus.Entry
	port int
}

func NewAPIServer(ctx context.Context, log *logrus.Entry, port int) *apiServer {
	return &apiServer{
		ctx:  ctx,
		log:  log,
		port: port,
	}
}

func (a *apiServer) Start(backend backend.Backend) error {
	logrus.Infof("Version: %s", version.Get())

	router := mux.NewRouter().StrictSlash(true)
	router.Use(loggingMiddleware(a.log))
	h := newHandler(backend)

	// When functioning properly, these routes will return the version of tha app that is running
	router.Path("/").HandlerFunc(h.root)
	router.Path("/healthz").HandlerFunc(h.root)

	api := router.PathPrefix("/v1").Subrouter()

	// POSTing to a domain creates a new domain and token. Further requests (below) against the created domain resource
	// require authentication using the token
	api.Path("/domains").Methods("POST").HandlerFunc(h.createDomain)

	// All routes using this authedRoutes subrouter will require token based authentication
	authedRoutes := api.PathPrefix("/domains/{domain}").Subrouter()
	authedRoutes.Use(tokenAuthMiddleware(backend))

	// Basic routes for the domain resource
	authedRoutes.Methods("GET").HandlerFunc(h.getDomain)

	// These are for records sub-resource
	authedRoutes.Path("/records").Methods("POST").HandlerFunc(h.createRecord)
	authedRoutes.Path("/records/{record}").Methods("DELETE").HandlerFunc(h.deleteRecord)

	// These are "actions" that can be taken on a domain
	authedRoutes.Path("/renew").Methods("POST").HandlerFunc(h.renew)
	authedRoutes.Path("/purgerecords").Methods("POST").HandlerFunc(h.purgerecords)

	// Note: this allows not found urls to be logged via the middleware
	// It **HAS** to be defined after all other paths are defined.
	router.NotFoundHandler = router.NewRoute().HandlerFunc(http.NotFound).GetHandler()

	// Below this point is where the server is started and graceful shutdown occurs.
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.port),
		Handler: ghandlers.CORS()(router),
	}

	go func() {
		a.log.WithField("port", a.port).Info("starting api server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.log.Fatalf("listen: %s\n", err)
		}
	}()

	go backend.StartPurgerDaemon(a.ctx.Done())

	<-a.ctx.Done()

	a.log.Info("shutting down the api server gracefully")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err := srv.Shutdown(ctx); err != nil {
		a.log.WithError(err).Error("unable to shutdown the api server gracefully")
		return err
	}

	return nil
}
