package relay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	defaultTimeout = 2 * time.Second
	defaultQOS     = byte(1)
)

type config struct {
	Broker    string
	Username  string
	Password  string
	ClientID  string
	CmdTopic  string
	RespTopic string
	QOS       byte
	Timeout   time.Duration
}

type RuntimeConfig struct {
	Broker    string `json:"broker"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
	ClientID  string `json:"client_id"`
	CmdTopic  string `json:"cmd_topic"`
	RespTopic string `json:"resp_topic"`
	QOS       byte   `json:"qos"`
	TimeoutMS int    `json:"timeout_ms"`
}

type Status struct {
	Configured bool              `json:"configured"`
	Valid      bool              `json:"valid"`
	Missing    []string          `json:"missing"`
	Error      string            `json:"error,omitempty"`
	Values     map[string]string `json:"values"`
}

func Enabled() bool {
	if Disabled() {
		return false
	}
	cfg := loadConfig()
	return cfg.Broker != "" && cfg.CmdTopic != ""
}

func Disabled() bool {
	return isTruthy(configValue("MQTT_RELAY_DISABLED", "MQTT_DISABLED"))
}

func Required() bool {
	if Disabled() {
		return false
	}
	return isTruthy(configValue("MQTT_RELAY_REQUIRED"))
}

func ConfigTruthy(keys ...string) bool {
	return isTruthy(configValue(keys...))
}

func CurrentStatus(validate bool) Status {
	disabled := Disabled()
	cfg := loadConfig()
	status := Status{
		Values: map[string]string{
			"broker":     cfg.Broker,
			"username":   cfg.Username,
			"client_id":  cfg.ClientID,
			"cmd_topic":  cfg.CmdTopic,
			"resp_topic": cfg.RespTopic,
			"qos":        strconv.Itoa(int(cfg.QOS)),
			"timeout_ms": strconv.Itoa(int(cfg.Timeout / time.Millisecond)),
			"disabled":   strconv.FormatBool(disabled),
		},
	}
	if cfg.Password != "" {
		status.Values["password_set"] = "true"
	} else {
		status.Values["password_set"] = "false"
	}

	if disabled {
		status.Configured = false
		status.Valid = true
		return status
	}

	status.Missing = missingFields(cfg)
	status.Configured = len(status.Missing) == 0
	if !status.Configured {
		return status
	}

	if validate {
		if err := ValidateConfig(cfg); err != nil {
			status.Error = err.Error()
			return status
		}
	}

	status.Valid = true
	return status
}

func SaveRuntimeConfig(input RuntimeConfig) Status {
	current := loadConfig()
	cfg := config{
		Broker:    strings.TrimSpace(input.Broker),
		Username:  strings.TrimSpace(input.Username),
		Password:  input.Password,
		ClientID:  strings.TrimSpace(input.ClientID),
		CmdTopic:  strings.TrimSpace(input.CmdTopic),
		RespTopic: strings.TrimSpace(input.RespTopic),
		QOS:       input.QOS,
		Timeout:   time.Duration(input.TimeoutMS) * time.Millisecond,
	}

	if cfg.Password == "" {
		cfg.Password = current.Password
	}
	if cfg.QOS > 2 {
		cfg.QOS = defaultQOS
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}

	status := Status{Missing: missingFields(cfg)}
	status.Configured = len(status.Missing) == 0
	if !status.Configured {
		return status
	}

	if err := ValidateConfig(cfg); err != nil {
		status.Error = err.Error()
		return status
	}

	if err := writeRuntimeEnv(cfg); err != nil {
		status.Error = err.Error()
		return status
	}

	return CurrentStatus(false)
}

func DisableRuntimeConfig() error {
	values := readRuntimeEnv()
	values["MQTT_RELAY_DISABLED"] = "true"
	values["MQTT_RELAY_REQUIRED"] = "false"
	return writeRuntimeValues(values)
}

func SetRelay(relayNumber uint8, on bool) error {
	if relayNumber == 0 {
		return nil
	}
	if relayNumber > 8 {
		return fmt.Errorf("relay number must be 1-8, got %d", relayNumber)
	}
	if Disabled() {
		return nil
	}

	cfg := loadConfig()
	if cfg.Broker == "" || cfg.CmdTopic == "" {
		return nil
	}
	if cfg.ClientID == "" {
		return errors.New("MQTT_RELAY_CLIENT_ID is required when relay MQTT is configured")
	}

	payload, err := BuildModbusRelayCommand(relayNumber, on)
	if err != nil {
		return err
	}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetConnectTimeout(cfg.Timeout).
		SetWriteTimeout(cfg.Timeout).
		SetAutoReconnect(false)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(cfg.Timeout) {
		return errors.New("mqtt connect timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt connect failed: %w", err)
	}
	defer client.Disconnect(250)

	token = client.Publish(cfg.CmdTopic, cfg.QOS, false, payload)
	if !token.WaitTimeout(cfg.Timeout) {
		return errors.New("mqtt publish timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt publish failed: %w", err)
	}

	return nil
}

func ValidateConfig(cfg config) error {
	if missing := missingFields(cfg); len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(fmt.Sprintf("%s-setup-%d", cfg.ClientID, time.Now().UnixNano())).
		SetConnectTimeout(cfg.Timeout).
		SetWriteTimeout(cfg.Timeout).
		SetAutoReconnect(false)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(cfg.Timeout) {
		return errors.New("mqtt connect timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt connect failed: %w", err)
	}
	defer client.Disconnect(250)

	response := make(chan struct{}, 1)
	token = client.Subscribe(cfg.RespTopic, cfg.QOS, func(_ mqtt.Client, _ mqtt.Message) {
		select {
		case response <- struct{}{}:
		default:
		}
	})
	if !token.WaitTimeout(cfg.Timeout) {
		return errors.New("mqtt subscribe timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt subscribe failed: %w", err)
	}

	payload := BuildModbusReadRelayStatusCommand()
	token = client.Publish(cfg.CmdTopic, cfg.QOS, false, payload)
	if !token.WaitTimeout(cfg.Timeout) {
		return errors.New("mqtt validate publish timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt validate publish failed: %w", err)
	}

	select {
	case <-response:
		return nil
	case <-time.After(cfg.Timeout):
		return errors.New("relay board did not respond")
	}
}

func BuildModbusRelayCommand(relayNumber uint8, on bool) ([]byte, error) {
	if relayNumber == 0 || relayNumber > 8 {
		return nil, fmt.Errorf("relay number must be 1-8, got %d", relayNumber)
	}

	address := uint16(relayNumber - 1)
	valueHigh := byte(0x00)
	valueLow := byte(0x00)
	if on {
		valueHigh = 0xFF
	}

	frame := []byte{
		0x01,
		0x05,
		byte(address >> 8),
		byte(address),
		valueHigh,
		valueLow,
	}
	crc := modbusCRC(frame)

	return append(frame, byte(crc), byte(crc>>8)), nil
}

func BuildModbusReadRelayStatusCommand() []byte {
	frame := []byte{0x01, 0x01, 0x00, 0x00, 0x00, 0x08}
	crc := modbusCRC(frame)
	return append(frame, byte(crc), byte(crc>>8))
}

func loadConfig() config {
	fileValues := readRuntimeEnv()
	return config{
		Broker:    firstValue(fileValues, "MQTT_RELAY_BROKER", "MQTT_BROKER"),
		Username:  firstValue(fileValues, "MQTT_RELAY_USERNAME", "MQTT_USERNAME"),
		Password:  firstValue(fileValues, "MQTT_RELAY_PASSWORD", "MQTT_PASSWORD"),
		ClientID:  firstValue(fileValues, "MQTT_RELAY_CLIENT_ID", "MQTT_CLIENT_ID"),
		CmdTopic:  firstValue(fileValues, "MQTT_RELAY_CMD_TOPIC", "MQTT_RELAY_TOPIC"),
		RespTopic: firstValue(fileValues, "MQTT_RELAY_RESP_TOPIC", "MQTT_RESP_TOPIC"),
		QOS:       loadQOS(),
		Timeout:   loadTimeout(),
	}
}

func missingFields(cfg config) []string {
	var missing []string
	required := map[string]string{
		"broker":     cfg.Broker,
		"username":   cfg.Username,
		"password":   cfg.Password,
		"client_id":  cfg.ClientID,
		"cmd_topic":  cfg.CmdTopic,
		"resp_topic": cfg.RespTopic,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func firstValue(fileValues map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fileValues[key]); value != "" {
			return value
		}
	}
	return firstEnv(keys...)
}

func configValue(keys ...string) string {
	return firstValue(readRuntimeEnv(), keys...)
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func loadQOS() byte {
	fileValues := readRuntimeEnv()
	value := firstValue(fileValues, "MQTT_RELAY_QOS", "MQTT_QOS")
	if value == "" {
		return defaultQOS
	}

	qos, err := strconv.Atoi(value)
	if err != nil || qos < 0 || qos > 2 {
		return defaultQOS
	}
	return byte(qos)
}

func loadTimeout() time.Duration {
	fileValues := readRuntimeEnv()
	value := firstValue(fileValues, "MQTT_RELAY_TIMEOUT_MS", "MQTT_TIMEOUT_MS")
	if value == "" {
		return defaultTimeout
	}

	timeoutMS, err := strconv.Atoi(value)
	if err != nil || timeoutMS <= 0 {
		return defaultTimeout
	}
	return time.Duration(timeoutMS) * time.Millisecond
}

func runtimeEnvPath() string {
	if path := strings.TrimSpace(os.Getenv("SITE_ENV_PATH")); path != "" {
		return path
	}
	return filepath.Join("config", "site.env")
}

func readRuntimeEnv() map[string]string {
	values := map[string]string{}
	data, err := os.ReadFile(runtimeEnvPath())
	if err != nil {
		return values
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return values
}

func writeRuntimeEnv(cfg config) error {
	values := readRuntimeEnv()
	delete(values, "MQTT_RELAY_REQUIRED")
	values["MQTT_RELAY_DISABLED"] = "false"
	values["MQTT_RELAY_BROKER"] = cfg.Broker
	values["MQTT_RELAY_USERNAME"] = cfg.Username
	values["MQTT_RELAY_PASSWORD"] = cfg.Password
	values["MQTT_RELAY_CLIENT_ID"] = cfg.ClientID
	values["MQTT_RELAY_CMD_TOPIC"] = cfg.CmdTopic
	values["MQTT_RELAY_RESP_TOPIC"] = cfg.RespTopic
	values["MQTT_RELAY_QOS"] = strconv.Itoa(int(cfg.QOS))
	values["MQTT_RELAY_TIMEOUT_MS"] = strconv.Itoa(int(cfg.Timeout / time.Millisecond))
	return writeRuntimeValues(values)
}

func writeRuntimeValues(values map[string]string) error {
	path := runtimeEnvPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", key, values[key]))
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600)
}

func modbusCRC(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&0x0001 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}
