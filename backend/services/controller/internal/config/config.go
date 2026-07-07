package config

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const LOCAL_ENV = ".env.local"

type Nats struct {
	Url       string
	Name      string
	EnableTls bool
	Cert      Tls
	Ctx       context.Context
}

type Mongo struct {
	Uri string
	Ctx context.Context
}

type RestApi struct {
	Port string
	Ctx  context.Context
}

type Controller struct {
	ControllerId string
}

type CampaignScheduler struct {
	Enabled  bool
	Interval time.Duration
}

type LockScale struct {
	RedisURL      string
	RedisEnabled  bool
	KafkaBrokers  string
	KafkaTopic    string
	KafkaEnabled  bool
	GreenplumDSN  string
	GreenplumEnabled bool
}

type LockRetryScheduler struct {
	Enabled       bool
	Interval      time.Duration
	CommandTimeout time.Duration
	MaxAttempts   int
}

type Config struct {
	RestApi            RestApi
	Nats               Nats
	Mongo              Mongo
	Controller         Controller
	CampaignScheduler  CampaignScheduler
	LockScale          LockScale
	LockRetryScheduler LockRetryScheduler
}

type Tls struct {
	CertFile string
	KeyFile  string
	CaFile   string
}

func NewConfig() *Config {

	loadEnvVariables()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	natsUrl := flag.String("nats_url", lookupEnvOrString("NATS_URL", "nats://localhost:4222"), "url for nats server")
	natsName := flag.String("nats_name", lookupEnvOrString("NATS_NAME", "controller"), "name for nats client")
	natsEnableTls := flag.Bool("nats_enable_tls", lookupEnvOrBool("NATS_ENABLE_TLS", false), "enbale TLS to nats server")
	clientCrt := flag.String("client_crt", lookupEnvOrString("CLIENT_CRT", "cert.pem"), "client certificate file to TLS connection")
	clientKey := flag.String("client_key", lookupEnvOrString("CLIENT_KEY", "key.pem"), "client key file to TLS connection")
	serverCA := flag.String("server_ca", lookupEnvOrString("SERVER_CA", "rootCA.pem"), "server CA file to TLS connection")
	flApiPort := flag.String("api_port", lookupEnvOrString("REST_API_PORT", "8000"), "Rest api port")
	mongoUri := flag.String("mongo_uri", lookupEnvOrString("MONGO_URI", "mongodb://localhost:27017"), "uri for mongodb server")
	controllerId := flag.String("controller_id", lookupEnvOrString("CONTROLLER_ID", "oktopusController"), "usp controller endpoint id")
	campaignSchedulerEnabled := flag.Bool("campaign_scheduler_enabled", lookupEnvOrBool("CAMPAIGN_SCHEDULER_ENABLED", true), "enable automatic campaign batch at time window start")
	campaignSchedulerIntervalSec := flag.Int("campaign_scheduler_interval_sec", lookupEnvOrInt("CAMPAIGN_SCHEDULER_INTERVAL_SEC", 60), "campaign scheduler tick interval in seconds (min 30)")
	lockRedisURL := flag.String("lock_redis_url", lookupEnvOrString("LOCK_REDIS_URL", ""), "redis URL for optional lock policy cache")
	lockRedisEnabled := flag.Bool("lock_redis_enabled", lookupEnvOrBool("LOCK_REDIS_ENABLED", false), "enable redis lock policy cache")
	lockKafkaBrokers := flag.String("lock_kafka_brokers", lookupEnvOrString("LOCK_KAFKA_BROKERS", ""), "comma-separated kafka brokers for lock audit stream")
	lockKafkaTopic := flag.String("lock_kafka_topic", lookupEnvOrString("LOCK_KAFKA_AUDIT_TOPIC", "ont-lock-audit"), "kafka topic for lock audit events")
	lockKafkaEnabled := flag.Bool("lock_kafka_enabled", lookupEnvOrBool("LOCK_KAFKA_ENABLED", false), "enable kafka lock audit producer")
	lockGreenplumDSN := flag.String("lock_greenplum_dsn", lookupEnvOrString("LOCK_GREENPLUM_DSN", ""), "postgres/greenplum DSN for lock audit sink")
	lockGreenplumEnabled := flag.Bool("lock_greenplum_enabled", lookupEnvOrBool("LOCK_GREENPLUM_ENABLED", false), "enable greenplum lock audit sink")
	lockRetryEnabled := flag.Bool("lock_retry_scheduler_enabled", lookupEnvOrBool("LOCK_RETRY_SCHEDULER_ENABLED", true), "enable lock command retry scheduler")
	lockRetryIntervalSec := flag.Int("lock_retry_scheduler_interval_sec", lookupEnvOrInt("LOCK_RETRY_SCHEDULER_INTERVAL_SEC", 30), "lock retry scheduler tick interval in seconds (min 15)")
	lockCommandTimeoutSec := flag.Int("lock_command_timeout_sec", lookupEnvOrInt("LOCK_COMMAND_TIMEOUT_SEC", 30), "minimum age before retrying a failed lock command")
	lockMaxAttempts := flag.Int("lock_command_max_attempts", lookupEnvOrInt("LOCK_COMMAND_MAX_ATTEMPTS", 10), "maximum lock command delivery attempts per command record")
	flHelp := flag.Bool("help", false, "Help")

	/*
		App variables priority:
		1º - Flag through command line.
		2º - Env variables.
		3º - Default flag value.
	*/

	flag.Parse()

	if *flHelp {
		flag.Usage()
		os.Exit(0)
	}

	ctx := context.TODO()

	return &Config{
		RestApi: RestApi{
			Port: *flApiPort,
			Ctx:  ctx,
		},
		Nats: Nats{
			Url:       *natsUrl,
			Name:      *natsName,
			EnableTls: *natsEnableTls,
			Ctx:       ctx,
			Cert: Tls{
				CertFile: *clientCrt,
				KeyFile:  *clientKey,
				CaFile:   *serverCA,
			},
		},
		Mongo: Mongo{
			Uri: *mongoUri,
			Ctx: ctx,
		},
		Controller: Controller{
			ControllerId: *controllerId,
		},
		CampaignScheduler: CampaignScheduler{
			Enabled:  *campaignSchedulerEnabled,
			Interval: time.Duration(*campaignSchedulerIntervalSec) * time.Second,
		},
		LockScale: LockScale{
			RedisURL:         *lockRedisURL,
			RedisEnabled:     *lockRedisEnabled,
			KafkaBrokers:     *lockKafkaBrokers,
			KafkaTopic:       *lockKafkaTopic,
			KafkaEnabled:     *lockKafkaEnabled,
			GreenplumDSN:     *lockGreenplumDSN,
			GreenplumEnabled: *lockGreenplumEnabled,
		},
		LockRetryScheduler: LockRetryScheduler{
			Enabled:        *lockRetryEnabled,
			Interval:       time.Duration(*lockRetryIntervalSec) * time.Second,
			CommandTimeout: time.Duration(*lockCommandTimeoutSec) * time.Second,
			MaxAttempts:    *lockMaxAttempts,
		},
	}
}

func loadEnvVariables() {
	err := godotenv.Load()

	if _, err := os.Stat(LOCAL_ENV); err == nil {
		_ = godotenv.Overload(LOCAL_ENV)
		log.Printf("Loaded variables from '%s'", LOCAL_ENV)
		return
	}

	if err != nil {
		log.Println("Error to load environment variables:", err)
	} else {
		log.Println("Loaded variables from '.env'")
	}
}

func lookupEnvOrString(key string, defaultVal string) string {
	if val, _ := os.LookupEnv(key); val != "" {
		return val
	}
	return defaultVal
}

func lookupEnvOrBool(key string, defaultVal bool) bool {
	if val, _ := os.LookupEnv(key); val != "" {
		v, err := strconv.ParseBool(val)
		if err != nil {
			log.Fatalf("LookupEnvOrBool[%s]: %v", key, err)
		}
		return v
	}
	return defaultVal
}

func lookupEnvOrInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		v, err := strconv.Atoi(val)
		if err != nil {
			log.Fatalf("lookupEnvOrInt[%s]: %v", key, err)
		}
		return v
	}
	return defaultVal
}
