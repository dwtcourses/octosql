package logical

import (
	"context"
	"reflect"
	"testing"

	"github.com/cube2222/octosql"
	"github.com/cube2222/octosql/execution"
	"github.com/cube2222/octosql/physical"
	"github.com/cube2222/octosql/physical/metadata"
)

type StubNode struct {
	metadata  *metadata.NodeMetadata
	variables octosql.Variables
}

func NewStubNode(metadata *metadata.NodeMetadata, variables octosql.Variables) *StubNode {
	return &StubNode{
		metadata:  metadata,
		variables: variables,
	}
}

func (sb *StubNode) Physical(ctx context.Context, physicalCreator *PhysicalPlanCreator) ([]physical.Node, octosql.Variables, error) {
	return []physical.Node{physical.NewStubNode(sb.metadata)}, sb.variables, nil
}

func TestJoin_Physical(t *testing.T) {
	type fields struct {
		source   Node
		joined   Node
		joinType execution.JoinType
	}
	tests := []struct {
		name     string
		fields   fields
		wantNode physical.Node
		wantErr  bool
	}{
		{
			name: "two unbounded streams - stream join",
			fields: fields{
				source: &StubNode{
					metadata: metadata.NewNodeMetadata(
						metadata.Unbounded,
						"",
						metadata.NewNamespace(
							[]string{""},
							[]octosql.VariableName{"a.field1", "a.field2"},
						),
					),
					variables: octosql.NoVariables(),
				},

				joined: &Filter{
					formula: &InfixOperator{ // ON a.field1 = b.field2 AND b.field1 = a.field2
						Left: &Predicate{
							Left:     &Variable{"a.field1"},
							Relation: Equal,
							Right:    &Variable{"b.field2"},
						},
						Operator: "and",
						Right: &Predicate{
							Left:     &Variable{"b.field1"},
							Relation: Equal,
							Right:    &Variable{"a.field2"},
						},
					},
					source: &StubNode{
						metadata: metadata.NewNodeMetadata(
							metadata.Unbounded,
							"",
							metadata.NewNamespace(
								[]string{""},
								[]octosql.VariableName{"b.field1", "b.field2"},
							),
						),
					},
				},

				joinType: execution.LEFT_JOIN,
			},

			wantNode: &physical.StreamJoin{
				SourceKey:      []physical.Expression{physical.NewVariable("a.field1"), physical.NewVariable("a.field2")},
				JoinedKey:      []physical.Expression{physical.NewVariable("b.field2"), physical.NewVariable("b.field1")},
				EventTimeField: "",
			},

			wantErr: false,
		},

		{
			name: "outer join - stream join",
			fields: fields{
				source: &StubNode{
					metadata: metadata.NewNodeMetadata(
						metadata.Unbounded,
						"",
						metadata.NewNamespace(
							[]string{""},
							[]octosql.VariableName{"a.field1"},
						),
					),
					variables: octosql.NoVariables(),
				},

				joined: &Filter{
					formula: &Predicate{ // ON a.field1 = b.field1
						Left:     &Variable{"a.field1"},
						Relation: Equal,
						Right:    &Variable{"b.field1"},
					},

					source: &StubNode{
						metadata: metadata.NewNodeMetadata(
							metadata.Unbounded,
							"",
							metadata.NewNamespace(
								[]string{""},
								[]octosql.VariableName{"b.field1"},
							),
						),
					},
				},

				joinType: execution.OUTER_JOIN,
			},

			wantNode: &physical.StreamJoin{
				SourceKey:      []physical.Expression{physical.NewVariable("a.field1")},
				JoinedKey:      []physical.Expression{physical.NewVariable("b.field1")},
				EventTimeField: "",
			},

			wantErr: false,
		},

		{
			name: "unbounded and fits in local storage - stream join + eventTimeField",
			fields: fields{
				source: &StubNode{
					metadata: metadata.NewNodeMetadata(
						metadata.Unbounded,
						"a.field1",
						metadata.NewNamespace(
							[]string{""},
							[]octosql.VariableName{"a.field1"},
						),
					),
					variables: octosql.NoVariables(),
				},

				joined: &Filter{
					formula: &Predicate{ // ON a.field1 = b.field1
						Left:     &Variable{"a.field1"},
						Relation: Equal,
						Right:    &Variable{"b.field1"},
					},

					source: &StubNode{
						metadata: metadata.NewNodeMetadata(
							metadata.BoundedFitsInLocalStorage,
							"b.field1",
							metadata.NewNamespace(
								[]string{""},
								[]octosql.VariableName{"b.field1"},
							),
						),
					},
				},

				joinType: execution.INNER_JOIN,
			},

			wantNode: &physical.StreamJoin{
				SourceKey:      []physical.Expression{physical.NewVariable("a.field1")},
				JoinedKey:      []physical.Expression{physical.NewVariable("b.field1")},
				EventTimeField: "a.field1",
			},

			wantErr: false,
		},

		{
			name: "two bounded streams, no predicate - stream join",
			fields: fields{
				source: &StubNode{
					metadata: metadata.NewNodeMetadata(
						metadata.Unbounded,
						"",
						metadata.NewNamespace(
							[]string{""},
							[]octosql.VariableName{"a.field1"},
						),
					),
					variables: octosql.NoVariables(),
				},

				joined: &StubNode{
					metadata: metadata.NewNodeMetadata(
						metadata.BoundedFitsInLocalStorage,
						"",
						metadata.NewNamespace(
							[]string{""},
							[]octosql.VariableName{"b.field1"},
						),
					),
				},

				joinType: execution.INNER_JOIN,
			},

			wantNode: &physical.StreamJoin{
				SourceKey:      []physical.Expression{},
				JoinedKey:      []physical.Expression{},
				EventTimeField: "",
			},

			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &Join{
				source:   tt.fields.source,
				joined:   tt.fields.joined,
				joinType: tt.fields.joinType,
			}

			gotNodes, _, err := node.Physical(context.Background(), NewPhysicalPlanCreator(nil, nil))
			if (err != nil) != tt.wantErr {
				t.Errorf("Physical() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr incorrect")
			}

			switch gotNode := gotNodes[0].(type) {
			case *physical.StreamJoin:
				wantNode, ok := tt.wantNode.(*physical.StreamJoin)

				if !ok {
					t.Errorf("Expected a lookup join else, got stream join")
				}

				if gotNode.EventTimeField != wantNode.EventTimeField {
					t.Errorf("Different event time fields: %v, %v", gotNode.EventTimeField, wantNode.EventTimeField)
				}

				if !reflect.DeepEqual(gotNode.SourceKey, wantNode.SourceKey) {
					t.Errorf("Different source key")
				}

				if !reflect.DeepEqual(gotNode.JoinedKey, wantNode.JoinedKey) {
					t.Errorf("Different joined key")
				}

				if gotNode.JoinType != tt.fields.joinType {
					t.Errorf("Invalid join type")
				}

			case *physical.LookupJoin:
				_, ok := tt.wantNode.(*physical.LookupJoin)
				if !ok {
					t.Errorf("Expected a stream join, got a lookup join")
				}
			default:
				panic("invalid type after join.Physical()")
			}

		})
	}
}

func Test_isConjunctionOfEqualities(t *testing.T) {
	type args struct {
		f physical.Formula
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "fail - OR",
			args: args{
				f: physical.NewOr(
					physical.NewConstant(true),
					physical.NewPredicate(
						physical.NewVariable("a"),
						physical.Equal,
						physical.NewVariable("b"),
					),
				),
			},
			want: false,
		},
		{
			name: "fail - negation",
			args: args{
				f: physical.NewAnd(
					physical.NewNot(
						physical.NewConstant(true),
					),
					physical.NewPredicate(
						physical.NewVariable("a"),
						physical.Equal,
						physical.NewVariable("b"),
					),
				),
			},
			want: false,
		},
		{
			name: "fail - predicate with inequality",
			args: args{
				f: physical.NewAnd(
					physical.NewConstant(true),
					physical.NewPredicate(
						physical.NewVariable("a"),
						physical.LessThan,
						physical.NewVariable("b"),
					),
				),
			},
			want: false,
		},
		{
			name: "fail - false constant",
			args: args{
				f: physical.NewAnd(
					physical.NewConstant(false),
					physical.NewPredicate(
						physical.NewVariable("a"),
						physical.Equal,
						physical.NewVariable("b"),
					),
				),
			},
			want: false,
		},
		{
			name: "pass",
			args: args{
				f: physical.NewAnd(
					physical.NewConstant(true),
					physical.NewAnd(
						physical.NewPredicate(
							physical.NewVariable("a"),
							physical.Equal,
							physical.NewVariable("b"),
						),
						physical.NewPredicate(
							physical.NewVariable("y"),
							physical.Equal,
							physical.NewVariable("x"),
						),
					),
				),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConjunctionOfEqualities(tt.args.f); got != tt.want {
				t.Errorf("isConjunctionOfEqualities() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getKeysFromFormula(t *testing.T) {
	type args struct {
		formula         physical.Formula
		sourceNamespace *metadata.Namespace
		joinedNamespace *metadata.Namespace
	}
	tests := []struct {
		name          string
		args          args
		wantSourceKey []physical.Expression
		wantJoinedKey []physical.Expression
		wantErr       bool
	}{
		{
			name: "empty formula",
			args: args{
				formula:         physical.NewConstant(true),
				sourceNamespace: metadata.NewNamespace(nil, nil),
				joinedNamespace: metadata.NewNamespace(nil, nil),
			},
			wantSourceKey: []physical.Expression{},
			wantJoinedKey: []physical.Expression{},
			wantErr:       false,
		},

		{
			name: "single correct predicate",
			args: args{
				formula: physical.NewPredicate(
					physical.NewVariable("source"),
					physical.Equal,
					physical.NewVariable("joined"),
				),
				sourceNamespace: metadata.NewNamespace(nil, []octosql.VariableName{"source"}),
				joinedNamespace: metadata.NewNamespace(nil, []octosql.VariableName{"joined"}),
			},
			wantSourceKey: []physical.Expression{physical.NewVariable("source")},
			wantJoinedKey: []physical.Expression{physical.NewVariable("joined")},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceKey, joinedKey, err := getKeysFromFormula(tt.args.formula, tt.args.sourceNamespace, tt.args.joinedNamespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("getKeysFromFormula() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(sourceKey, tt.wantSourceKey) {
				t.Errorf("getKeysAndEventTimeFromFormula() sourceKey = %v, want %v", sourceKey, tt.wantSourceKey)
			}

			if !reflect.DeepEqual(joinedKey, tt.wantJoinedKey) {
				t.Errorf("getKeysAndEventTimeFromFormula() joinedKey = %v, want %v", joinedKey, tt.wantJoinedKey)
			}
		})
	}
}