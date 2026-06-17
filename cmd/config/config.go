package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"text/tabwriter"
	"time"

	"github.com/scality/go-errors"
	"github.com/sethvargo/go-envconfig"
)

// ErrProcessEnv is returned when environment variables cannot be processed.
var ErrProcessEnv = errors.New("failed to process environment variables")

const ApplicationName = "prometheus-postgres-adapter"

//nolint:gochecknoglobals // This global will be overwritten at build time
var ApplicationVersion = "dev"

type (
	Environment struct {
		LoggerLogLevel string `env:"LOGGER_LOG_LEVEL, default=info"`

		HTTP     HTTP       `env:",prefix=HTTP_"`
		GRPC     GRPC       `env:",prefix=GRPC_"`
		StoreAPI StoreAPI   `env:",prefix=STORE_API_"`
		Database PostgreSQL `env:",prefix=POSTGRESQL_DATABASE_"`

		MetricParserCount int `env:"METRIC_PARSER_COUNT, default=1"`
		MetricWriterCount int `env:"METRIC_WRITER_COUNT, default=1"`
	}

	PostgreSQL struct {
		SSLMode  string `env:"SSL_MODE"`
		Name     string `env:"NAME"`
		User     string `env:"USER"`
		Password string `env:"PASSWORD" secret:"true"`
		Host     string `env:"HOST"`
		Port     int    `env:"PORT"`
	}

	HTTP struct {
		Addr string `env:"ADDR, default=:9201"`
	}

	GRPC struct {
		Addr string `env:"ADDR, default=:10901"`
	}

	StoreAPI struct {
		// ExternalLabels are advertised to Thanos via the StoreAPI Info call and
		// used for deduplication. Format: "key1:value1,key2:value2".
		ExternalLabels map[string]string `env:"EXTERNAL_LABELS"`
	}
)

func NewEnvironment(ctx context.Context) (*Environment, error) {
	cfg := &Environment{}

	err := envconfig.Process(ctx, cfg)
	if err != nil {
		return nil, errors.Wrap(ErrProcessEnv, errors.CausedBy(err))
	}

	fmt.Println(ToString(cfg))

	return cfg, nil
}

func ToString(src any) string {
	b := &bytes.Buffer{}

	writer := tabwriter.NewWriter(b, 0, 0, 1, ' ', tabwriter.Debug)
	write(writer, src, 0)
	_ = writer.Flush()

	return b.String()
}

//nolint:gocognit // This will be moved to a library at some point
func write(writer io.Writer, src any, level int) {
	value := reflect.ValueOf(src)

	if value.Kind() != reflect.Struct {
		value = value.Elem()
	}

	prefix := ""
	for range level {
		prefix += "  "
	}

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)

		if !field.CanInterface() {
			continue
		}

		typeField := value.Type().Field(i)

		if field.Kind() == reflect.Struct && field.Type() != reflect.TypeOf(time.Time{}) {
			_, _ = fmt.Fprintf(writer, "%s%s:\t\t\n", prefix, typeField.Name)
			write(writer, field.Interface(), level+1)

			continue
		}

		val := field.Interface()
		if v, ok := typeField.Tag.Lookup("secret"); ok && v == "true" {
			val = "********"
		}

		_, _ = fmt.Fprintf(writer, "%s%s\t %v\t %s\n", prefix, typeField.Name, val, field.Type().String())
	}
}
