package analyzer

import (
	"github.com/src-d/go-mysql-server/sql"
	"github.com/src-d/go-mysql-server/sql/expression"
	"github.com/src-d/go-mysql-server/sql/plan"
)

func resolveStar(ctx *sql.Context, a *Analyzer, n sql.Node) (sql.Node, error) {
	span, _ := ctx.Span("resolve_star")
	defer span.Finish()

	a.Log("resolving star, node of type: %T", n)
	return plan.TransformUp(n, func(n sql.Node) (sql.Node, error) {
		if n.Resolved() {
			return n, nil
		}

		switch n := n.(type) {
		case *plan.Project:
			if !n.Child.Resolved() {
				return n, nil
			}

			expressions, err := expandStars(a, n.Projections, n.Child.Schema())
			if err != nil {
				return nil, err
			}

			return plan.NewProject(expressions, n.Child), nil
		case *plan.GroupBy:
			if !n.Child.Resolved() {
				return n, nil
			}

			aggregate, err := expandStars(a, n.Aggregate, n.Child.Schema())
			if err != nil {
				return nil, err
			}

			return plan.NewGroupBy(aggregate, n.Grouping, n.Child), nil
		default:
			return n, nil
		}
	})
}

func expandStars(a *Analyzer, exprs []sql.Expression, schema sql.Schema) ([]sql.Expression, error) {
	var expressions []sql.Expression
	for _, e := range exprs {
		if s, ok := e.(*expression.Star); ok {
			var exprs []sql.Expression
			for i, col := range schema {
				if s.Table == "" || s.Table == col.Source {
					exprs = append(exprs, expression.NewGetFieldWithTable(
						i, col.Type, col.Source, col.Name, col.Nullable,
					))
				}
			}

			if len(exprs) == 0 && s.Table != "" {
				return nil, sql.ErrTableNotFound.New(s.Table)
			}

			expressions = append(expressions, exprs...)
		} else {
			expressions = append(expressions, e)
		}
	}

	a.Log("resolved * to expressions %s", expressions)
	return expressions, nil
}
