package health

import (
	"SuperBizAgent/utility/common"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
	amqp "github.com/rabbitmq/amqp091-go"
)

const dependencyCheckTimeout = 3 * time.Second

var errCheckSkipped = errors.New("check skipped")

type MilvusCollectionReport struct {
	Collection string
	SchemaOK   bool
	DocCount   int64
}

var (
	CloseAllMilvusClientsFunc   func() error
	InspectMilvusCollectionFunc func(ctx context.Context) (MilvusCollectionReport, error)
	MilvusReadyCheckFunc        func(ctx context.Context) error
	CloseMySQLFunc              func() error
)

type CheckStatus struct {
	Ready      bool   `json:"ready"`
	Error      string `json:"error,omitempty"`
	Skipped    bool   `json:"skipped,omitempty"`
	Collection string `json:"collection,omitempty"`
	SchemaOK   *bool  `json:"schema_ok,omitempty"`
	DocCount   *int64 `json:"doc_count,omitempty"`
}

type ReadinessReport struct {
	Ready  bool                   `json:"ready"`
	Checks map[string]CheckStatus `json:"checks"`
}

var (
	redisReadyCheck    = defaultRedisReadyCheck
	milvusReadyCheck   = injectedMilvusReadyCheck
	rabbitMQReadyCheck = defaultRabbitMQReadyCheck
)

var knowledgeReadyCheck = defaultKnowledgeReadyCheck

func injectedMilvusReadyCheck(parent context.Context) error {
	if _, ok := milvusAddress(parent); !ok {
		return errCheckSkipped
	}
	if MilvusReadyCheckFunc == nil {
		return errCheckSkipped
	}
	ctx, cancel := context.WithTimeout(parent, dependencyCheckTimeout)
	defer cancel()
	return MilvusReadyCheckFunc(ctx)
}

func BuildReadinessReport(ctx context.Context, shuttingDown bool) (ReadinessReport, int) {
	checks := map[string]CheckStatus{
		"server": {Ready: !shuttingDown},
	}
	ready := !shuttingDown
	if shuttingDown {
		checks["server"] = CheckStatus{
			Ready: false,
			Error: "shutdown in progress",
		}
	}

	for _, probe := range []struct {
		name string
		fn   func(context.Context) error
	}{
		{name: "redis", fn: redisReadyCheck},
		{name: "milvus", fn: milvusReadyCheck},
		{name: "rabbitmq", fn: rabbitMQReadyCheck},
	} {
		err := probe.fn(ctx)
		switch {
		case err == nil:
			checks[probe.name] = CheckStatus{Ready: true}
		case errors.Is(err, errCheckSkipped):
			checks[probe.name] = CheckStatus{Ready: true, Skipped: true}
		default:
			ready = false
			checks[probe.name] = CheckStatus{
				Ready: false,
				Error: err.Error(),
			}
		}
	}
	checks["knowledge"] = knowledgeReadyCheck(ctx)

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	return ReadinessReport{
		Ready:  ready,
		Checks: checks,
	}, status
}

func defaultKnowledgeReadyCheck(parent context.Context) CheckStatus {
	addr, ok := milvusAddress(parent)
	if !ok {
		return CheckStatus{Ready: true, Skipped: true}
	}
	_ = addr

	if InspectMilvusCollectionFunc == nil {
		return CheckStatus{Ready: true, Skipped: true}
	}

	ctx, cancel := context.WithTimeout(parent, dependencyCheckTimeout)
	defer cancel()

	report, err := InspectMilvusCollectionFunc(ctx)
	schemaOK := report.SchemaOK
	docCount := report.DocCount
	status := CheckStatus{
		Ready:      true,
		Collection: report.Collection,
		SchemaOK:   &schemaOK,
		DocCount:   &docCount,
	}
	if err != nil {
		status.Error = err.Error()
	}
	return status
}

func CloseResources(ctx context.Context) error {
	var errs []string

	if hasRedisConfig(ctx) {
		if err := g.Redis().Close(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("redis close failed: %v", err))
		}
	}
	if hasMySQLConfig(ctx) && CloseMySQLFunc != nil {
		if err := CloseMySQLFunc(); err != nil {
			errs = append(errs, fmt.Sprintf("mysql close failed: %v", err))
		}
	}
	if CloseAllMilvusClientsFunc != nil {
		if err := CloseAllMilvusClientsFunc(); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func defaultRedisReadyCheck(parent context.Context) error {
	if !hasRedisConfig(parent) {
		return errCheckSkipped
	}

	ctx, cancel := context.WithTimeout(parent, dependencyCheckTimeout)
	defer cancel()

	result, err := g.Redis().Do(ctx, "PING")
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	if !strings.EqualFold(result.String(), "PONG") {
		return fmt.Errorf("unexpected ping response: %s", result.String())
	}
	return nil
}

func defaultRabbitMQReadyCheck(parent context.Context) error {
	if !rabbitMQEnabled(parent) {
		return errCheckSkipped
	}

	url, ok := rabbitMQURL(parent)
	if !ok {
		return fmt.Errorf("rabbitmq.url is not configured")
	}

	conn, err := amqp.DialConfig(url, amqp.Config{
		Dial: amqp.DefaultDial(dependencyCheckTimeout),
	})
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel failed: %w", err)
	}
	defer ch.Close()

	return nil
}

func hasRedisConfig(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "redis.default.address")
	if err != nil {
		return false
	}
	_, ok := common.ResolveOptionalEnv(v.String())
	return ok
}

func hasMySQLConfig(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "mysql.dsn")
	if err != nil {
		return false
	}
	_, ok := common.ResolveOptionalEnv(v.String())
	return ok
}

func milvusAddress(ctx context.Context) (string, bool) {
	v, err := g.Cfg().Get(ctx, "milvus.address")
	if err != nil {
		return "", false
	}
	return common.ResolveOptionalEnv(v.String())
}

func rabbitMQEnabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "rabbitmq.enabled")
	if err != nil {
		return false
	}
	return v.Bool()
}

func rabbitMQURL(ctx context.Context) (string, bool) {
	v, err := g.Cfg().Get(ctx, "rabbitmq.url")
	if err != nil {
		return "", false
	}
	return common.ResolveOptionalEnv(v.String())
}
