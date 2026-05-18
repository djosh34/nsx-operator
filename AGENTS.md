Never skip tests, never skip error of anything (no _ := ... for when _ is error)
Log almost everything in debug logging, and larger actions in info.
use zap logging library, always do structured logging in jsonl to stderr

To test against nsx-t, use mockapi from ../nsx-t-mockapi (you can use testcontainers)