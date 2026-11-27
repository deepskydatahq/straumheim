package pubsub

// Observer receives bounded-label request delivery outcomes.
type Observer interface {
	RecordPubSubPublish(protocol, result string)
	RecordPubSubPush(result string)
}

const (
	resultSuccess   = "success"
	resultFailure   = "failure"
	resultMalformed = "malformed"
)
