package config

// Default returns a fully populated config. It must return fresh slices on
// every call: the YAML decoder replaces slices wholesale, so sharing a
// package-level slice would let one loaded file mutate another's defaults.
func Default() Config {
	return Config{
		Northbound: Northbound{
			MQTT: MQTTConfig{
				ClientID:   "collector-{node.id}",
				QoS:        1,
				KeepaliveS: 60,
				Reconnect:  ReconnectConfig{MinBackoffS: 1, MaxBackoffS: 30},
			},
		},
		Adapter: Adapter{
			Name: "irrigation-v1",
			Topics: Topics{
				PropertyPost:  "v1/{product_key}/{device_sn}/property/post",
				ServiceReply:  "v1/{product_key}/{device_sn}/service/reply",
				Event:         "v1/{product_key}/{device_sn}/event/{event_id}",
				ServiceInvoke: "v1/{product_key}/{device_sn}/service/invoke",
				NodeState:     "v1/{product_key}/$nodes/{node_id}/state",
			},
		},
		Timing: Timing{
			SnapshotIntervalS:     60,
			DeltaDebounceMS:       2000,
			FreshnessWindowS:      180,
			OfflineAfterIntervals: 3,
			ReadDeadlineMS:        300,
			WriteVerifyDeadlineMS: 500,
			CmdTimeoutMS:          600000,
			CmdRetryMax:           3,
			LoraAckWindows:        2,
			LoraWindowMarginS:     30,
		},
		Cache: Cache{
			EventQueueMax:       1000,
			EventMaxBytes:       2048,
			EventDroppedReserve: 1,
		},
		Queue: Queue{PendingMax: 4},
		State: State{
			DBPath:              "/var/lib/collect-code/state.db",
			IdempotencyTTLHours: 168,
			CompactionIntervalS: 3600,
		},
		Log: LogConfig{
			Level:        "info",
			Format:       "json",
			RedactFields: []string{"password", "token", "authorization", "data"},
		},
		Health: Health{
			Enabled: false,
			Addr:    "127.0.0.1:9100",
		},
	}
}
