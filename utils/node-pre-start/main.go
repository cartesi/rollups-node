package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	redis "github.com/redis/go-redis/v9"

	"github.com/avast/retry-go"
)

type nodePreStartConfig struct {
	chainID                string
	contractAddress        string
	postgresEndpoint       *url.URL
	sunodoValidatorEnabled bool
	redisEndpoint          *url.URL
}

func createPostgresDB(cfg nodePreStartConfig) error {
	err := retry.Do(
		func() error {
			db, err := sqlx.Connect("postgres", cfg.postgresEndpoint.String())
			if err != nil {
				log.Println("ERR: POSTGRESQL: can't connect: ", err, "")
				return err
			}
			defer db.Close()

			if err := db.Ping(); err != nil {
				log.Println("ERR: POSTGRESQL: can't ping: ", err, "")
				return err
			}

			// TODO: move to a different naming like $chainID_$contractAddress in the future
			// dbName := fmt.Sprintf("%s_%s", cfg.chainID, cfg.contractAddress)
			var dbName = cfg.contractAddress
			row := db.QueryRow("SELECT 1 FROM pg_database WHERE datname=$1", dbName)
			if err := row.Scan(); errors.Is(err, sql.ErrNoRows) {
				// Database does not exist, create it
				_, err := db.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName))
				if err != nil {
					log.Println("ERR: POSTGRESQL: can't create database: ", err, "")
					return err
				}
				log.Printf("POSTGRESQL: Database %s created", dbName)
			} else {
				log.Printf("POSTGRESQL: Database %s already exists", dbName)
			}

			return nil
		},
	)

	if err != nil {
		return err
	}

	return nil
}

func loadConfig() (nodePreStartConfig, error) {
	var cfg = nodePreStartConfig{}

	// global configuration
	chainID, exists := os.LookupEnv("CARTESI_BLOCKCHAIN_ID")
	if !exists {
		return nodePreStartConfig{}, errors.New("missing environment variable CARTESI_BLOCKCHAIN_ID")
	}
	cfg.chainID = chainID
	log.Printf("CONFIG: CARTESI_BLOCKCHAIN_ID=%s", chainID)

	contractAddress, exists := os.LookupEnv("CARTESI_CONTRACTS_APPLICATION_ADDRESS")
	if !exists {
		return nodePreStartConfig{}, errors.New("missing environment variable CARTESI_CONTRACTS_APPLICATION_ADDRESS")
	}
	cfg.contractAddress = strings.ToLower(contractAddress)
	log.Printf("CONFIG: CARTESI_CONTRACTS_APPLICATION_ADDRESS=%s", contractAddress)

	// postgres configuration
	_postgresEndpoint, exists := os.LookupEnv("CARTESI_POSTGRES_ENDPOINT")
	if !exists {
		return nodePreStartConfig{}, errors.New("missing environment variable CARTESI_POSTGRES_ENDPOINT")
	}
	postgresEndpoint, err := url.Parse(_postgresEndpoint)
	if err != nil {
		log.Fatalln("ERR: ", err)
		return nodePreStartConfig{}, err
	}
	cfg.postgresEndpoint = postgresEndpoint
	log.Printf("CARTESI_POSTGRES_ENDPOINT=%s://%s:[REDACTED]@%s/\n", postgresEndpoint.Scheme, postgresEndpoint.User.Username(), postgresEndpoint.Host)

	// redis configuration
	_sunodoValidatorEnabled, exists := os.LookupEnv("CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_ENABLED")
	if exists {
		sunodoValidatorEnabled, err := strconv.ParseBool(_sunodoValidatorEnabled)
		if err != nil {
			return nodePreStartConfig{}, err
		}

		if sunodoValidatorEnabled {
			cfg.sunodoValidatorEnabled = true
			_redisEndpoint, exists := os.LookupEnv("CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_REDIS_ENDPOINT")
			if exists {
				redisEndpoint, err := url.Parse(_redisEndpoint)
				if err != nil {
					return nodePreStartConfig{}, err
				}
				cfg.redisEndpoint = redisEndpoint
				log.Printf("CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_REDIS_ENDPOINT=%s://%s", redisEndpoint.Scheme, redisEndpoint.Host)
			}
		}
	} else {
		cfg.sunodoValidatorEnabled = false
	}

	return cfg, nil
}

func cleanupRedisStreams(cfg nodePreStartConfig) error {
	ctx := context.Background()

	err := retry.Do(
		func() error {
			// Clean inputs and outputs streams
			rdb := redis.NewClient(&redis.Options{
				Addr:     cfg.redisEndpoint.Host,
				Password: cfg.redisEndpoint.User.String(),
				DB:       0, // use default DB
			})

			_, err := rdb.Ping(ctx).Result()
			if err != nil {
				log.Fatalln("ERR: Redis connection failed: ", err, "")
				return err
			}

			defer rdb.Close()

			var INPUTS_STREAM = fmt.Sprintf("{chain-%s:dapp-%s}:rollups-inputs", cfg.chainID, cfg.contractAddress[2:])
			var OUTPUTS_STREAM = fmt.Sprintf("{chain-%s:dapp-%s}:rollups-outputs", cfg.chainID, cfg.contractAddress[2:])

			_, err = rdb.Do(ctx, "DEL", INPUTS_STREAM).Result()
			if err != nil {
				return err
			}

			_, err = rdb.Do(ctx, "DEL", OUTPUTS_STREAM).Result()
			if err != nil {
				return err
			}

			log.Println("REDIS: Cleaned inputs and outputs streams.")

			return nil
		},
	)

	if err != nil {
		return err
	}

	return nil

}

func main() {
	var cfg nodePreStartConfig

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalln(err, " service=node-pre-start")
	}

	err = createPostgresDB(cfg)
	if err != nil {
		log.Fatalln(err, " service=node-pre-start")
	}

	// only clean redis streams if sunodo validator is enabled
	if cfg.sunodoValidatorEnabled && cfg.redisEndpoint.String() != "" {
		err = cleanupRedisStreams(cfg)
		if err != nil {
			log.Fatalln(err, " service=node-pre-start")
		}
	}
}
