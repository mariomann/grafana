package ngalert

import (
	"context"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/grafana/grafana/pkg/services/ngalert/eval"

	"github.com/grafana/grafana/pkg/api/routing"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/registry"
	"github.com/grafana/grafana/pkg/services/datasources"
	"github.com/grafana/grafana/pkg/services/sqlstore"
	"github.com/grafana/grafana/pkg/services/sqlstore/migrator"
	"github.com/grafana/grafana/pkg/setting"
)

const (
	maxAttempts int64 = 3
	// defaultIntervalInSeconds is the default interval of each alert definition
	defaultIntervalInSeconds int64 = 60
	// baseInterval is the interval of the scheduler
	baseInterval time.Duration = 10 * time.Second
)

// AlertNG is the service for evaluating the condition of an alert definition.
type AlertNG struct {
	Cfg             *setting.Cfg             `inject:""`
	DatasourceCache datasources.CacheService `inject:""`
	RouteRegister   routing.RouteRegister    `inject:""`
	SQLStore        *sqlstore.SQLStore       `inject:""`
	log             log.Logger
	schedule        *schedule
}

func init() {
	registry.RegisterService(&AlertNG{})
}

// Init initializes the AlertingService.
func (ng *AlertNG) Init() error {
	ng.log = log.New("ngalert")

	ng.registerAPIEndpoints()
	return nil
}

// Run starts the scheduler
func (ng *AlertNG) Run(ctx context.Context) error {
	ng.log.Debug("ngalert starting")
	ng.schedule = newScheduler(clock.New(), baseInterval, ng.log, nil)
	return ng.alertingTicker(ctx)
}

// IsDisabled returns true if the alerting service is disable for this instance.
func (ng *AlertNG) IsDisabled() bool {
	if ng.Cfg == nil {
		return false
	}
	// Check also about expressions?
	return !ng.Cfg.IsNgAlertEnabled()
}

// AddMigration defines database migrations.
// If Alerting NG is not enabled does nothing.
func (ng *AlertNG) AddMigration(mg *migrator.Migrator) {
	if ng.IsDisabled() {
		return
	}
	addAlertDefinitionMigrations(mg)
	addAlertDefinitionVersionMigrations(mg)
}

// LoadAlertCondition returns a Condition object for the given alertDefinitionID.
func (ng *AlertNG) LoadAlertCondition(alertDefinitionID int64) (*eval.Condition, error) {
	getAlertDefinitionByIDQuery := getAlertDefinitionByIDQuery{ID: alertDefinitionID}
	if err := ng.getAlertDefinitionByID(&getAlertDefinitionByIDQuery); err != nil {
		return nil, err
	}
	alertDefinition := getAlertDefinitionByIDQuery.Result

	err := ng.validateAlertDefinition(alertDefinition, true)
	if err != nil {
		return nil, err
	}

	return &eval.Condition{
		RefID:                 alertDefinition.Condition,
		OrgID:                 alertDefinition.OrgID,
		QueriesAndExpressions: alertDefinition.Data,
	}, nil
}
