package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExternalUserMapping stores integration user identity mappings.
type ExternalUserMapping struct {
	ent.Schema
}

func (ExternalUserMapping) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "external_user_mappings"},
	}
}

func (ExternalUserMapping) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (ExternalUserMapping) Fields() []ent.Field {
	return []ent.Field{
		field.String("external_user_id").
			MaxLen(255).
			NotEmpty(),
		field.Int64("user_id"),
		field.Int64("api_key_id"),
		field.String("username_snapshot").
			MaxLen(100).
			Default(""),
	}
}

func (ExternalUserMapping) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("external_user_mappings").
			Field("user_id").
			Required().
			Unique(),
		edge.From("api_key", APIKey.Type).
			Ref("external_user_mappings").
			Field("api_key_id").
			Required().
			Unique(),
	}
}

func (ExternalUserMapping) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("external_user_id"),
		index.Fields("user_id"),
		index.Fields("api_key_id"),
	}
}
