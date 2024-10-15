/* Service template with:
 * - cancellation
 * - readiness check
 * - liveliness check
 * - logging
 * - polling
 * - reloading
 * - signal handling
 */
package claimer

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lmittmann/tint"

	signtx "github.com/cartesi/rollups-node/internal/aws-kms-signtx"
	conf "github.com/cartesi/rollups-node/internal/config"
	repo "github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_cfg "github.com/aws/aws-sdk-go-v2/config"
	aws_kms "github.com/aws/aws-sdk-go-v2/service/kms"
)

const (
	/* Name is used to identify this service on log messages and on
	 * the health checks endpoints */
	Name = "claimer"
)

/* Configuration values to Create this service. */
type CreateInfo struct {
	LogLevel                 string
	PollInterval             time.Duration
	Context                  context.Context
	SignalTrapsEnabled       bool
	HealthChecksEnabled      bool
	HealthCheckAddress       string
	HealthCheckRetryInterval time.Duration

	Auth                   conf.Auth
	BlockchainHttpEndpoint conf.Redacted[string]
	EthConn                ethclient.Client
	PostgresEndpoint       conf.Redacted[string]
}

/* Runtime values to run this service. */
type Service struct {
	logLevel                 slog.Level
	ticker                  *time.Ticker
	context                  context.Context
	cancel                   context.CancelFunc
	sighup                   chan os.Signal // SIGHUP to reload
	sigint                   chan os.Signal // SIGINT to exit gracefully
	running                  atomic.Bool
	httpMux                  *http.ServeMux
	healthServer             *http.Server
	healthServerFunc         func()
	healthCheckRetryInterval time.Duration

	consensus              iconsensus.IConsensus
	dbConn                *repo.Database
	ethConn               *ethclient.Client
	signer                *bind.TransactOpts
}

////////////////////////////////////////////////////////////////////////////////
// Service
////////////////////////////////////////////////////////////////////////////////

/* Create the service using CreateInfo as the configuration.
 * An empty config should still create a functioning server if possible. */
func Create(ci CreateInfo) (s *Service, e error) {
	var err error
	s = &Service{}

	// log
	s.logLevel = map[string]slog.Level {
		"debug" : slog.LevelDebug,
		"info"  : slog.LevelInfo,
		"warn"  : slog.LevelWarn,
		"error" : slog.LevelError,
	}[ci.LogLevel]

	if ci.LogLevel == "" {
		s.logLevel = slog.LevelDebug
	}
	opts := &tint.Options{
		Level:      s.logLevel,
		AddSource:  s.logLevel == slog.LevelDebug,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
	}
	handler := tint.NewHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// poller
	if ci.PollInterval == 0 {
		ci.PollInterval = 5000 * time.Millisecond
	}
	s.ticker = time.NewTicker(ci.PollInterval)

	// cancelation
	if ci.Context == nil {
		ci.Context = context.Background()
	}
	s.context, s.cancel = context.WithCancel(ci.Context)

	// signal handlers
	if ci.SignalTrapsEnabled {
		s.sighup = make(chan os.Signal, 1)
		signal.Notify(s.sighup, syscall.SIGHUP)

		s.sigint = make(chan os.Signal, 1)
		signal.Notify(s.sigint, syscall.SIGINT)
	}

	// health endpoints
	if ci.HealthChecksEnabled {
		if ci.HealthCheckAddress == "" {
			ci.HealthCheckAddress = ":80"
		}
		if ci.HealthCheckRetryInterval == 0 {
			ci.HealthCheckRetryInterval = 5 * time.Second
		}
		s.httpMux = http.NewServeMux()
		s.healthServer, s.healthServerFunc =
			s.StartHealthServer(ci.HealthCheckAddress, 3,
			ci.HealthCheckRetryInterval, s.httpMux)
	}

	// blockchain
	s.ethConn, err = ethclient.Dial(ci.BlockchainHttpEndpoint.Value)
	if err != nil {
		return nil, err
	}
	chainid, err := s.ethConn.NetworkID(ci.Context)
	if err != nil {
		return nil, err
	}
	s.signer, err = createSigner(ci.Context, ci.Auth, chainid)
	if err != nil {
		return nil, err
	}

	// db
	s.dbConn, err = repo.Connect(ci.Context, ci.PostgresEndpoint.Value)
	if err != nil {
		return nil, err
	}

	slog.Info("create",
		"service",   Name,
		"pid",       os.Getpid(),
		"log-level", s.logLevel)
	return s, nil
}

/* Start the service in:
 * - serve=true blocking or,
 * - serve=false non-blocking mode */
func (s *Service) Start(serve bool) {
	slog.Info("start", "service", Name)

	go s.healthServerFunc()
	s.running.Store(true)
	if serve {
		s.loop()
	} else {
		go s.loop()
	}
}

/* Stop the service in:
 * - force=true forcefully or,
 * - force=false gracefully by giving it time to shutdown its components */
func (s *Service) Stop(force bool) {
	slog.Info("stop", "service", Name, "force", force)

	s.running.Store(false)
	s.healthServer.Close()
	if force {
		s.cancel()
	}
}

/* Reload behavior is service specific. A common, expected use is to
 * reconfigure a running service. */
func (s *Service) Reload() {
	slog.Info("reload", "service", Name)
}

/* Service is Ready */
func (s *Service) Ready() bool {
	running := s.running.Load()

	slog.Info("checkready", "service", Name, "ready", running)
	return running
}

/* Service is Alive */
func (s *Service) Alive() bool {
	alive := true

	slog.Info("checkalive", "service", Name, "alive", alive)
	return alive
}

func (s *Service) tick() error {
	claims, err := s.dbConn.GetComputedClaims(s.context)
	if err != nil {
		return err
	}

	slog.Info("tick", "service", Name, "claim_computed", len(claims))
	for _, claim := range(claims) {
		// TODO: detect duplicates?

		slog.Info("tick:submit_claim", "service", Name, "claim", claim)
		instance, err := iconsensus.NewIConsensus(claim.IConsensusAddress, s.ethConn)
		if err != nil {
			return err
		}

		tx, err := instance.SubmitClaim(s.signer, claim.AppAddress, claim.LastBlock, claim.Hash)
		if err != nil {
			return err
		}

		slog.Info("tick:update_database", "service", Name, "id", claim.ID, "transaction_hash", tx.Hash())
		err = s.dbConn.SetClaimAsSubmitted(s.context, claim.ID, tx.Hash())
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) loop() {
	for s.running.Load() {
		select {
		case <-s.sighup:
			s.Reload()
		case <-s.sigint:
			s.Stop(false)
		case <-s.context.Done():
			s.Stop(true)
		case <-s.ticker.C:
			err := s.tick()
			if err != nil {
				slog.Error("tick", "service", Name, "cause", err)
			}
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// health checks
////////////////////////////////////////////////////////////////////////////////

/* Create `/Name/readyz` and `/Name/livez` HTTP endpoints on
 * addr. Recreate the HTTP server up to maxRetries in case it goes down
 * unexpectedly waiting for retryInterval beteen each attempt. Rounte the
 * endpoints with mux. */
func (s *Service) StartHealthServer(
	addr string,
	maxRetries int,
	retryInterval time.Duration,
	mux *http.ServeMux,
) (*http.Server, func()) {
	mux.Handle("/"+Name+"/readyz", http.HandlerFunc(s.ReadyHandler))
	mux.Handle("/"+Name+"/livez", http.HandlerFunc(s.AliveHandler))
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return server, func() {
		for retry := 0; retry < maxRetries+1; retry++ {
			slog.Info("http", "service", Name, "addr", addr)
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				slog.Error("http",
					"service", Name,
					"error", err,
					"try", retry+1,
					"maxRetries", maxRetries)
			}
			time.Sleep(retryInterval)
		}
	}
}

/* HTTP handler for `/Name/readyz` that exposes the value of Ready() */
func (s *Service) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if !s.Ready() {
		http.Error(w, Name+": ready check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, Name+": ready\n")
	}
}

/* HTTP handler for `/Name/livez` that exposes the value of Alive() */
func (s *Service) AliveHandler(w http.ResponseWriter, r *http.Request) {
	if !s.Alive() {
		http.Error(w, Name+": alive check failed",
			http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, Name+": alive\n")
	}
}

func createSigner(ctx context.Context, auth conf.Auth, chainID *big.Int) (*bind.TransactOpts, error) {
	switch auth := auth.(type) {
	case conf.AuthMnemonic:
		privateKey, err := ethutil.MnemonicToPrivateKey(
			auth.Mnemonic.Value, uint32(auth.AccountIndex.Value))
		if err != nil {
			return nil, err
		}
		return bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	case conf.AuthAWS:
		awsc, err := aws_cfg.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, err
		}
		kms := aws_kms.NewFromConfig(awsc, func(o *aws_kms.Options) {
			o.BaseEndpoint = aws.String(auth.EndpointURL.Value)
		})
		return signtx.CreateAWSTransactOpts(ctx, kms,
			aws.String(auth.KeyID.Value), types.NewEIP155Signer(chainID))
	}
	return nil, fmt.Errorf("error: unimplemented authentication method")
}
