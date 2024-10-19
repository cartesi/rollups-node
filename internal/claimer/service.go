// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_cfg "github.com/aws/aws-sdk-go-v2/config"
	aws_kms "github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	/* Name is used to identify this service on log messages and on
	 * the health checks endpoints */
	Name = "template"
)

/* Configuration values to Create this service. */
type CreateInfo struct {
	LogLevel                 string
	PollInterval             time.Duration
	Context                  context.Context
	SignalTrapsEnabled       bool
	TelemetryEnabled         bool
	TelemetryAddress         string
	TelemetryRetryInterval   time.Duration

	Auth                   conf.Auth
	BlockchainHttpEndpoint conf.Redacted[string]
	EthConn                ethclient.Client
	PostgresEndpoint       conf.Redacted[string]
}

/* Service metrics */
type Metrics struct {
	tickCount prometheus.Counter
	duplicate prometheus.Counter
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
	telemetryServer          *http.Server
	telemetryServerFunc       func()

	metrics                   Metrics
	//ci      CreateInfo // maybe needed for reload

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

	// telemetry endpoints
	if ci.TelemetryEnabled {
		if ci.TelemetryAddress == "" {
			ci.TelemetryAddress = ":80"
		}
		if ci.TelemetryRetryInterval == 0 {
			ci.TelemetryRetryInterval = 5 * time.Second
		}
		s.httpMux = http.NewServeMux()
		s.telemetryServer, s.telemetryServerFunc =
			s.StartTelemetryServer(ci.TelemetryAddress, 3,
			ci.TelemetryRetryInterval, s.httpMux)
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

	go s.telemetryServerFunc()
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
	s.telemetryServer.Close()
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

/* submit a claim to the blockchain */
func (s *Service) submitClaimBlockchain(
	instance *iconsensus.IConsensus,
	claim    *repo.ComputedClaim,
	
) (*types.Transaction, error) {
	lastBlockNumber := new(big.Int).SetUint64(claim.LastBlock)
	tx, err := instance.SubmitClaim(s.signer,
		claim.AppAddress, lastBlockNumber, claim.Hash)
	if err != nil {
		slog.Error("submitClaimBlockchain:failed",
			"service",    Name,
			"appAddress", claim.AppAddress,
			"claimHash",  claim.Hash,
			"txHash",     tx.Hash(),
			"error",      err)
		return nil, err
	} else {
		slog.Info("SubmitClaimBlockchain:success",
			"service",    Name,
			"appAddress", claim.AppAddress,
			"claimHash",  claim.Hash,
			"TxHash",     tx.Hash())
	}
	return tx, nil
}

/* update the database epoch status to CLAIM_SUBMITTED and add a transaction hash */
func (s *Service) submitClaimDB(
	claim    *repo.ComputedClaim,
	txHash   common.Hash,
) error {
	err := s.dbConn.SetClaimAsSubmitted(s.context, claim.ID, txHash)
	if err != nil {
		slog.Error("submitClaimDB:failed",
			"service",    Name,
			"appAddress", claim.AppAddress,
			"hash",       claim.Hash,
			"txHash",     txHash,
			"error",      err)
		return err
	} else {
		slog.Info("submitClaimDB:success",
			"service",    Name,
			"appAddress", claim.AppAddress,
			"hash",       claim.Hash,
			"txHash",     txHash)
	}
	return nil
}

/* foreach computed claim:
 *     if already present on the blockchain => mark as duplicate
 *     otherwise => submit it */
func (s *Service) submitClaimsAndUpdateDatabase(claims []repo.ComputedClaim) error {
	if len(claims) == 0 {
		return nil
	}

	toClaim := make(map[common.Hash]*repo.ComputedClaim)
	for _, claim := range(claims) {
		toClaim[claim.Hash] = &claim
	}

	for i:=0; i < len(claims); i++ {
		instance, err := iconsensus.NewIConsensus(
			claims[i].IConsensusAddress, s.ethConn)
		if err != nil {
			return err
		}
		appAddress := claims[i].AppAddress
		start := claims[i].LastBlock
		it, err := instance.FilterClaimSubmission(&bind.FilterOpts{
			Context: s.context,
			Start: start,
		}, nil, nil)

		for it.Next() {
			event := it.Event
			if claim, ok := toClaim[event.Claim]; ok {
				slog.Warn("SubmitClaim:duplicate",
					"service",    Name,
					"AppAddress", claim.AppAddress,
					"Hash",       claim.Hash,
					"TxHash",     event.Raw.TxHash)

				s.dbConn.SetClaimAsSubmitted(s.context,
					claim.ID, event.Raw.TxHash)
				delete(toClaim, event.Claim)
				s.metrics.duplicate.Inc()
			}
		}

		for ; i < len(claims) && claims[i].AppAddress == appAddress; i++ {
			if claim, ok := toClaim[claims[i].Hash]; ok {
				tx, _ := s.submitClaimBlockchain(instance, claim)
				s.submitClaimDB(claim, tx.Hash())
				toClaim[claim.Hash] = nil
			}
		}
	}

	return nil
}

func (s *Service) tick() (int, error) {
	claims, err := s.dbConn.GetComputedClaims(s.context)
	if err != nil {
		return 0, err
	}
	return len(claims), s.submitClaimsAndUpdateDatabase(claims)
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
			start := time.Now();
			s.metrics.tickCount.Inc()
			computed, err := s.tick()
			if err != nil {
				slog.Error("tick",
					"service", Name,
					"cause", err)
			}
			slog.Info("tick",
				"service", Name,
				"computed", computed,
				"tick", 0,
				"time", time.Now().Sub(start))
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// telemetry (health, metrics)
////////////////////////////////////////////////////////////////////////////////

/* Create `/Name/readyz`, `/Name/livez` and `/Name/metrics` HTTP endpoints on
 * addr. Recreate the HTTP server up to maxRetries in case it goes down
 * unexpectedly waiting for retryInterval beteen each attempt. Route endpoints
 * with mux. */
func (s *Service) StartTelemetryServer(
	addr string,
	maxRetries int,
	retryInterval time.Duration,
	mux *http.ServeMux,
) (*http.Server, func()) {
	mux.Handle("/"+Name+"/readyz",  http.HandlerFunc(s.ReadyHandler))
	mux.Handle("/"+Name+"/livez",   http.HandlerFunc(s.AliveHandler))
	mux.Handle("/"+Name+"/metrics", promhttp.Handler())

	s.metrics.tickCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: Name + "_tick_total",
		Help: "total tick",
	})
	s.metrics.duplicate = promauto.NewCounter(prometheus.CounterOpts{
		Name: Name + "_duplicate_claim_total",
		Help: "total duplicate claims submitted",
	})

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
