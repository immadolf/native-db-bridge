package policy

import "testing"

func TestMongoAggregateStages(t *testing.T) {
	allowed := []string{"$match", "$project", "$limit", "$skip", "$sort", "$group", "$count", "$unwind"}
	for _, stage := range allowed {
		if !IsMongoAggregateStageAllowed(stage) {
			t.Fatalf("%s should be allowed", stage)
		}
	}
	rejected := []string{"$out", "$merge", "$function", "$accumulator", "$where", "$graphLookup", "$lookup"}
	for _, stage := range rejected {
		if IsMongoAggregateStageAllowed(stage) {
			t.Fatalf("%s must be rejected", stage)
		}
	}
}

func TestMongoWriteMatrix(t *testing.T) {
	cases := []struct {
		name         string
		operation    string
		hasFilter    bool
		hasDocument  bool
		hasDocuments bool
		want         bool
	}{
		{"insertOne ok", "insertOne", false, true, false, true},
		{"insertOne rejects filter", "insertOne", true, true, false, false},
		{"insertMany ok", "insertMany", false, false, true, true},
		{"updateOne ok", "updateOne", true, true, false, true},
		{"deleteMany ok", "deleteMany", true, false, false, true},
		{"dropCollection ok", "dropCollection", false, false, false, true},
		{"dropCollection rejects filter", "dropCollection", true, false, false, false},
		{"dropDatabase rejected", "dropDatabase", false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateMongoWrite(tc.operation, tc.hasFilter, tc.hasDocument, tc.hasDocuments)
			if got != tc.want {
				t.Fatalf("ValidateMongoWrite=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestMongoAggregateCaseInsensitive(t *testing.T) {
	if !IsMongoAggregateStageAllowed("$Match") {
		t.Fatalf("IsMongoAggregateStageAllowed('$Match') should be true")
	}
	if !IsMongoAggregateStageAllowed("$PROJECT") {
		t.Fatalf("IsMongoAggregateStageAllowed('$PROJECT') should be true")
	}
}

func TestMongoWriteUnknownOperation(t *testing.T) {
	if ValidateMongoWrite("unknownOp", false, false, false) {
		t.Fatalf("unknown operation should be rejected")
	}
}
