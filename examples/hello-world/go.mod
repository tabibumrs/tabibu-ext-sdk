module hello-world

go 1.24

require github.com/Nexus-Labs-254/tabibu-ext-sdk v0.0.0

require (
	github.com/BryanMwangi/pine v1.1.7 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/rabbitmq/amqp091-go v1.12.0 // indirect
	github.com/segmentio/kafka-go v0.4.47 // indirect
)

// Remove this replace directive once the SDK is published to GitHub.
replace github.com/Nexus-Labs-254/tabibu-ext-sdk => ../../
