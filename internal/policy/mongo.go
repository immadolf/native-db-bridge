package policy

import "strings"

// mongoAllowedAggregateStages is the whitelist of safe MongoDB aggregation
// pipeline stages.
var mongoAllowedAggregateStages = map[string]bool{
	"$match":   true,
	"$project": true,
	"$limit":   true,
	"$skip":    true,
	"$sort":    true,
	"$group":   true,
	"$count":   true,
	"$unwind":  true,
}

// IsMongoAggregateStageAllowed reports whether the given aggregation pipeline
// stage is in the safety whitelist. The stage name is compared
// case-insensitively.
func IsMongoAggregateStageAllowed(stage string) bool {
	return mongoAllowedAggregateStages[strings.ToLower(strings.TrimSpace(stage))]
}

// ValidateMongoWrite validates a MongoDB write operation against the write
// matrix. It returns true if the combination of operation and parameters is
// allowed, false otherwise.
//
// The write matrix enforces:
//   - insertOne: requires document, rejects filter
//   - insertMany: requires documents, rejects filter
//   - updateOne: requires filter and document
//   - updateMany: requires filter and document
//   - deleteOne: requires filter
//   - deleteMany: requires filter
//   - dropCollection: no parameters, rejects filter
//   - dropDatabase: no parameters, rejects filter
func ValidateMongoWrite(operation string, hasFilter, hasDocument, hasDocuments bool) bool {
	switch strings.TrimSpace(operation) {
	case "insertOne":
		// insertOne must have a document, must NOT have a filter
		return hasDocument && !hasDocuments && !hasFilter

	case "insertMany":
		// insertMany must have documents, must NOT have a filter
		return hasDocuments && !hasDocument && !hasFilter

	case "updateOne", "updateMany":
		// update requires filter and document
		return hasFilter && hasDocument && !hasDocuments

	case "deleteOne", "deleteMany":
		// delete requires filter, no document
		return hasFilter && !hasDocument && !hasDocuments

	case "dropCollection", "dropDatabase":
		// drop operations must have no parameters
		return !hasFilter && !hasDocument && !hasDocuments

	default:
		return false
	}
}
