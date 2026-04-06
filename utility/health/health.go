package health

import (
	"SuperBizAgent/internal/ai/tools"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
)

const dependencyCheckTimeout = 3 * time.Second

var errCheckSkipped = errors.New("check skipped")

type CheckStatus struct {
	Ready   bool   `json:"ready"`
	Error   string `json:"error,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
}

type ReadinessReport struct {
	Ready  bool                   `json:"ready"`
	Checks map[string]CheckStatus `json:"checks"`
}

var (
	redisReadyCheck  = defaultRedisReadyCheck
	milvusReadyCheck = defaultMilvusReadyCheck
)

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

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	return ReadinessReport{
		Ready:  ready,
		Checks: checks,
	}, status
}

func CloseResources(ctx context.Context) error {
	var errs []string

	if hasRedisConfig(ctx) {
		if err := g.Redis().Close(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("redis close failed: %v", err))
		}
	}
	if hasMySQLConfig(ctx) {
		if err := tools.CloseMySQL(); err != nil {
			errs = append(errs, fmt.Sprintf("mysql close failed: %v", err))
		}
	}
	if err := client.CloseAllMilvusClients(); err != nil {
		errs = append(errs, err.Error())
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

func defaultMilvusReadyCheck(parent context.Context) error {
	addr, ok := milvusAddress(parent)
	if !ok {
		return errCheckSkipped
	}

	ctx, cancel := context.WithTimeout(parent, dependencyCheckTimeout)
	defer cancel()

	cli, err := milvusclient.NewClient(ctx, milvusclient.Config{
		Address: addr,
		DBName:  "default",
	})
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer cli.Close()

	if _, err := cli.ListDatabases(ctx); err != nil {
		return fmt.Errorf("list databases failed: %w", err)
	}
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
