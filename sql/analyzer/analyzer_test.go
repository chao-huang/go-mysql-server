package analyzer

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dolthub/go-mysql-server/memory"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/expression"
	"github.com/dolthub/go-mysql-server/sql/plan"
)

func TestAnalyzer_Analyze(t *testing.T) {
	require := require.New(t)

	table := memory.NewPushdownTable("mytable", sql.Schema{
		{Name: "i", Type: sql.Int32, Source: "mytable"},
		{Name: "t", Type: sql.Text, Source: "mytable"},
	})

	table2 := memory.NewPushdownTable("mytable2", sql.Schema{
		{Name: "i2", Type: sql.Int32, Source: "mytable2"},
	})

	db := memory.NewDatabase("mydb")
	db.AddTable("mytable", table)
	db.AddTable("mytable2", table2)

	catalog := sql.NewCatalog()
	catalog.AddDatabase(db)
	a := withoutProcessTracking(NewDefault(catalog))

	var notAnalyzed sql.Node = plan.NewUnresolvedTable("mytable", "")
	ctx := sql.NewContext(context.Background(), sql.WithIndexRegistry(sql.NewIndexRegistry()), sql.WithViewRegistry(sql.NewViewRegistry())).WithCurrentDB("mydb")
	analyzed, err := a.Analyze(ctx, notAnalyzed, nil)
	require.NoError(err)
	require.Equal(
		plan.NewResolvedTable(table),
		analyzed,
	)

	notAnalyzed = plan.NewUnresolvedTable("nonexistant", "")
	analyzed, err = a.Analyze(ctx, notAnalyzed, nil)
	require.Error(err)
	require.NotNil(analyzed)

	analyzed, err = a.Analyze(ctx, plan.NewResolvedTable(table), nil)
	require.NoError(err)
	require.Equal(
		plan.NewResolvedTable(table),
		analyzed,
	)

	notAnalyzed = plan.NewProject(
		[]sql.Expression{expression.NewUnresolvedColumn("o")},
		plan.NewUnresolvedTable("mytable", ""),
	)
	_, err = a.Analyze(ctx, notAnalyzed, nil)
	require.Error(err)

	notAnalyzed = plan.NewProject(
		[]sql.Expression{expression.NewUnresolvedColumn("i")},
		plan.NewUnresolvedTable("mytable", ""),
	)
	analyzed, err = a.Analyze(ctx, notAnalyzed, nil)
	var expected sql.Node = plan.NewDecoratedNode("Projected table access on [i]", plan.NewResolvedTable(
		table.WithProjection([]string{"i"}),
	))
	require.NoError(err)
	assertNodesEqualWithDiff(t, expected, analyzed)

	notAnalyzed = plan.NewDescribe(
		plan.NewUnresolvedTable("mytable", ""),
	)
	analyzed, err = a.Analyze(ctx, notAnalyzed, nil)
	expected = plan.NewDescribe(
		plan.NewResolvedTable(table),
	)
	require.NoError(err)
	assertNodesEqualWithDiff(t, expected, analyzed)

	notAnalyzed = plan.NewProject(
		[]sql.Expression{expression.NewStar()},
		plan.NewUnresolvedTable("mytable", ""),
	)
	analyzed, err = a.Analyze(ctx, notAnalyzed, nil)
	require.NoError(err)

	expected = plan.NewDecoratedNode("Projected table access on [i t]", plan.NewResolvedTable( table.WithProjection([]string{"i", "t"})))
	assertNodesEqualWithDiff(t, expected, analyzed)

	notAnalyzed = plan.NewProject(
		[]sql.Expression{expression.NewStar()},
		plan.NewProject(
			[]sql.Expression{expression.NewStar()},
			plan.NewUnresolvedTable("mytable", ""),
		),
	)
	analyzed, err = a.Analyze(ctx, notAnalyzed, nil)
	require.NoError(err)

	expected = plan.NewDecoratedNode("Projected table access on [i t]", plan.NewResolvedTable(table.WithProjection([]string{"i", "t"})))
	assertNodesEqualWithDiff(t, expected, analyzed)

	notAnalyzed = plan.NewProject(
		[]sql.Expression{
			expression.NewAlias("foo", expression.NewUnresolvedColumn("i")),
		},
		plan.NewUnresolvedTable("mytable", ""),
	)
	analyzed, err = a.Analyze(ctx, notAnalyzed, nil)
	expected = plan.NewProject(
		[]sql.Expression{
			expression.NewAlias("foo", expression.NewGetFieldWithTable(0, sql.Int32, "mytable", "i", false)),
		},
		plan.NewDecoratedNode("Projected table access on [i]",
			plan.NewResolvedTable(table.WithProjection([]string{"i"}))),
	)
	require.NoError(err)
	assertNodesEqualWithDiff(t, expected, analyzed)

	notAnalyzed = plan.NewProject(
		[]sql.Expression{expression.NewUnresolvedColumn("i")},
		plan.NewFilter(
			expression.NewEquals(
				expression.NewUnresolvedColumn("i"),
				expression.NewLiteral(int32(1), sql.Int32),
			),
			plan.NewUnresolvedTable("mytable", ""),
		),
	)
	analyzed, err = a.Analyze(ctx, notAnalyzed, nil)
	expected = plan.NewDecoratedNode("Filtered table access on [mytable.i = 1]",
		plan.NewDecoratedNode("Projected table access on [i]",
			plan.NewResolvedTable(
				table.WithFilters([]sql.Expression{
					expression.NewEquals(
						expression.NewGetFieldWithTable(0, sql.Int32, "mytable", "i", false),
						expression.NewLiteral(int32(1), sql.Int32),
					),
				}).(*memory.PushdownTable).WithProjection([]string{"i"}),
			),
		),
	)
	require.NoError(err)
	assertNodesEqualWithDiff(t, expected, analyzed)

	notAnalyzed = plan.NewProject(
		[]sql.Expression{
			expression.NewUnresolvedColumn("i"),
			expression.NewUnresolvedColumn("i2"),
		},
		plan.NewCrossJoin(
			plan.NewUnresolvedTable("mytable", ""),
			plan.NewUnresolvedTable("mytable2", ""),
		),
	)
	analyzed, err = a.Analyze(ctx, notAnalyzed, nil)
	expected = plan.NewCrossJoin(
		plan.NewDecoratedNode("Projected table access on [i]", plan.NewResolvedTable(table.WithProjection([]string{"i"}))),
		plan.NewDecoratedNode("Projected table access on [i2]", plan.NewResolvedTable(table2.WithProjection([]string{"i2"}))),
	)
	require.NoError(err)
	assertNodesEqualWithDiff(t, expected, analyzed)

	notAnalyzed = plan.NewLimit(int64(1),
		plan.NewProject(
			[]sql.Expression{
				expression.NewUnresolvedColumn("i"),
			},
			plan.NewUnresolvedTable("mytable", ""),
		),
	)
	analyzed, err = a.Analyze(ctx, notAnalyzed, nil)
	expected = plan.NewLimit(
		int64(1),
		plan.NewDecoratedNode("Projected table access on [i]",
			plan.NewResolvedTable(table.WithProjection([]string{"i"}))),
	)
	require.NoError(err)
	assertNodesEqualWithDiff(t, expected, analyzed)
}

func TestMaxIterations(t *testing.T) {
	require := require.New(t)
	tName := "my-table"
	table := memory.NewTable(tName, sql.Schema{
		{Name: "i", Type: sql.Int32, Source: tName},
		{Name: "t", Type: sql.Text, Source: tName},
	})
	db := memory.NewDatabase("mydb")
	db.AddTable(tName, table)

	catalog := sql.NewCatalog()
	catalog.AddDatabase(db)

	count := 0
	a := withoutProcessTracking(NewBuilder(catalog).AddPostAnalyzeRule("loop",
		func(c *sql.Context, a *Analyzer, n sql.Node, scope *Scope) (sql.Node, error) {

			switch n.(type) {
			case *plan.ResolvedTable:
				count++
				name := fmt.Sprintf("mytable-%v", count)
				table := memory.NewTable(name, sql.Schema{
					{Name: "i", Type: sql.Int32, Source: name},
					{Name: "t", Type: sql.Text, Source: name},
				})
				n = plan.NewResolvedTable(table)
			}

			return n, nil
		}).Build())

	ctx := sql.NewContext(context.Background(), sql.WithIndexRegistry(sql.NewIndexRegistry()), sql.WithViewRegistry(sql.NewViewRegistry())).WithCurrentDB("mydb")
	notAnalyzed := plan.NewUnresolvedTable(tName, "")
	analyzed, err := a.Analyze(ctx, notAnalyzed, nil)
	require.NoError(err)
	require.Equal(
		plan.NewResolvedTable(
			memory.NewTable("mytable-1000", sql.Schema{
				{Name: "i", Type: sql.Int32, Source: "mytable-1000"},
				{Name: "t", Type: sql.Text, Source: "mytable-1000"},
			}),
		),
		analyzed,
	)
	require.Equal(1000, count)
}

func TestAddRule(t *testing.T) {
	require := require.New(t)

	defRulesCount := countRules(NewDefault(nil).Batches)

	a := NewBuilder(nil).AddPostAnalyzeRule("foo", pushdownFilters).Build()

	require.Equal(countRules(a.Batches), defRulesCount+1)
}

func TestAddPreValidationRule(t *testing.T) {
	require := require.New(t)

	defRulesCount := countRules(NewDefault(nil).Batches)

	a := NewBuilder(nil).AddPreValidationRule("foo", pushdownFilters).Build()

	require.Equal(countRules(a.Batches), defRulesCount+1)
}

func TestAddPostValidationRule(t *testing.T) {
	require := require.New(t)

	defRulesCount := countRules(NewDefault(nil).Batches)

	a := NewBuilder(nil).AddPostValidationRule("foo", pushdownFilters).Build()

	require.Equal(countRules(a.Batches), defRulesCount+1)
}

func countRules(batches []*Batch) int {
	var count int
	for _, b := range batches {
		count = count + len(b.Rules)
	}
	return count

}

func TestMixInnerAndNaturalJoins(t *testing.T) {
	var require = require.New(t)

	table := memory.NewPushdownTable("mytable", sql.Schema{
		{Name: "i", Type: sql.Int32, Source: "mytable"},
		{Name: "f", Type: sql.Float64, Source: "mytable"},
		{Name: "t", Type: sql.Text, Source: "mytable"},
	})

	table2 := memory.NewPushdownTable("mytable2", sql.Schema{
		{Name: "i2", Type: sql.Int32, Source: "mytable2"},
		{Name: "f2", Type: sql.Float64, Source: "mytable2"},
		{Name: "t2", Type: sql.Text, Source: "mytable2"},
	})

	table3 := memory.NewPushdownTable("mytable3", sql.Schema{
		{Name: "i", Type: sql.Int32, Source: "mytable3"},
		{Name: "f2", Type: sql.Float64, Source: "mytable3"},
		{Name: "t3", Type: sql.Text, Source: "mytable3"},
	})

	db := memory.NewDatabase("mydb")
	db.AddTable("mytable", table)
	db.AddTable("mytable2", table2)
	db.AddTable("mytable3", table3)

	catalog := sql.NewCatalog()
	catalog.AddDatabase(db)
	a := withoutProcessTracking(NewDefault(catalog))

	node := plan.NewProject(
		[]sql.Expression{
			expression.NewStar(),
		},
		plan.NewNaturalJoin(
			plan.NewInnerJoin(
				plan.NewUnresolvedTable("mytable", ""),
				plan.NewUnresolvedTable("mytable2", ""),
				expression.NewEquals(
					expression.NewUnresolvedQualifiedColumn("mytable", "i"),
					expression.NewUnresolvedQualifiedColumn("mytable2", "i2"),
				),
			),
			plan.NewUnresolvedTable("mytable3", ""),
		),
	)

	expected := plan.NewProject(
		[]sql.Expression{
			expression.NewGetFieldWithTable(0, sql.Int32, "mytable", "i", false),
			expression.NewGetFieldWithTable(3, sql.Float64, "mytable2", "f2", false),
			expression.NewGetFieldWithTable(1, sql.Float64, "mytable", "f", false),
			expression.NewGetFieldWithTable(2, sql.Text, "mytable", "t", false),
			expression.NewGetFieldWithTable(4, sql.Int32, "mytable2", "i2", false),
			expression.NewGetFieldWithTable(5, sql.Text, "mytable2", "t2", false),
			expression.NewGetFieldWithTable(6, sql.Text, "mytable3", "t3", false),
		},
		plan.NewInnerJoin(
			plan.NewInnerJoin(
				plan.NewDecoratedNode("Projected table access on [i f t]", plan.NewResolvedTable(table.WithProjection([]string{"i", "f", "t"}))),
				plan.NewDecoratedNode("Projected table access on [f2 i2 t2]", plan.NewResolvedTable(table2.WithProjection([]string{"f2", "i2", "t2"}))),
				expression.NewEquals(
					expression.NewGetFieldWithTable(0, sql.Int32, "mytable", "i", false),
					expression.NewGetFieldWithTable(4, sql.Int32, "mytable2", "i2", false),
				),
			),
			plan.NewDecoratedNode("Projected table access on [t3 i f2]", plan.NewResolvedTable(table3.WithProjection([]string{"t3", "i", "f2"}))),
			expression.NewAnd(
				expression.NewEquals(
					expression.NewGetFieldWithTable(0, sql.Int32, "mytable", "i", false),
					expression.NewGetFieldWithTable(7, sql.Int32, "mytable3", "i", false),
				),
				expression.NewEquals(
					expression.NewGetFieldWithTable(3, sql.Float64, "mytable2", "f2", false),
					expression.NewGetFieldWithTable(8, sql.Float64, "mytable3", "f2", false),
				),
			),
		),
	)

	ctx := sql.NewContext(context.Background(), sql.WithIndexRegistry(sql.NewIndexRegistry()), sql.WithViewRegistry(sql.NewViewRegistry())).WithCurrentDB("mydb")
	result, err := a.Analyze(ctx, node, nil)
	require.NoError(err)
	require.Equal(expected, result)
}

func TestReorderProjectionUnresolvedChild(t *testing.T) {
	require := require.New(t)
	node := plan.NewProject(
		[]sql.Expression{
			expression.NewUnresolvedQualifiedColumn("rc", "commit_hash"),
			expression.NewUnresolvedColumn("commit_author_when"),
		},
		plan.NewFilter(
			expression.JoinAnd(
				expression.NewEquals(
					expression.NewUnresolvedQualifiedColumn("rc", "repository_id"),
					expression.NewLiteral("foo", sql.LongText),
				),
				expression.NewEquals(
					expression.NewUnresolvedQualifiedColumn("rc", "ref_name"),
					expression.NewLiteral("HEAD", sql.LongText),
				),
				expression.NewEquals(
					expression.NewUnresolvedQualifiedColumn("rc", "history_index"),
					expression.NewLiteral(int64(0), sql.Int64),
				),
			),
			plan.NewNaturalJoin(
				plan.NewInnerJoin(
					plan.NewUnresolvedTable("refs", ""),
					plan.NewTableAlias("rc",
						plan.NewUnresolvedTable("ref_commits", ""),
					),
					expression.NewAnd(
						expression.NewEquals(
							expression.NewUnresolvedQualifiedColumn("refs", "ref_name"),
							expression.NewUnresolvedQualifiedColumn("rc", "ref_name"),
						),
						expression.NewEquals(
							expression.NewUnresolvedQualifiedColumn("refs", "repository_id"),
							expression.NewUnresolvedQualifiedColumn("rc", "repository_id"),
						),
					),
				),
				plan.NewTableAlias("c",
					plan.NewUnresolvedTable("commits", ""),
				),
			),
		),
	)

	commits := memory.NewTable("commits", sql.Schema{
		{Name: "repository_id", Source: "commits", Type: sql.Text},
		{Name: "commit_hash", Source: "commits", Type: sql.Text},
		{Name: "commit_author_when", Source: "commits", Type: sql.Text},
	})

	refs := memory.NewTable("refs", sql.Schema{
		{Name: "repository_id", Source: "refs", Type: sql.Text},
		{Name: "ref_name", Source: "refs", Type: sql.Text},
	})

	refCommits := memory.NewTable("ref_commits", sql.Schema{
		{Name: "repository_id", Source: "ref_commits", Type: sql.Text},
		{Name: "ref_name", Source: "ref_commits", Type: sql.Text},
		{Name: "commit_hash", Source: "ref_commits", Type: sql.Text},
		{Name: "history_index", Source: "ref_commits", Type: sql.Int64},
	})

	db := memory.NewDatabase("")
	db.AddTable("refs", refs)
	db.AddTable("ref_commits", refCommits)
	db.AddTable("commits", commits)

	catalog := sql.NewCatalog()
	catalog.AddDatabase(db)
	a := withoutProcessTracking(NewDefault(catalog))

	ctx := sql.NewContext(context.Background(), sql.WithIndexRegistry(sql.NewIndexRegistry()), sql.WithViewRegistry(sql.NewViewRegistry()))
	result, err := a.Analyze(ctx, node, nil)
	require.NoError(err)
	require.True(result.Resolved())
}
