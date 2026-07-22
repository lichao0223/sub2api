package service

import (
	"context"
	"strconv"
)

const (
	DefaultLoginIPBlockThreshold       = 5
	DefaultLoginIPBlockDurationSeconds = 1800
)

var loginIPBlockDurations = map[int]struct{}{
	0: {}, 1800: {}, 3600: {}, 21600: {}, 86400: {}, 604800: {},
}

type LoginIPBlockConfig struct {
	Enabled         bool
	Threshold       int
	DurationSeconds int
}

func (s *SettingService) GetLoginIPBlockConfig(ctx context.Context) (LoginIPBlockConfig, error) {
	config := LoginIPBlockConfig{
		Threshold:       DefaultLoginIPBlockThreshold,
		DurationSeconds: DefaultLoginIPBlockDurationSeconds,
	}
	if s == nil || s.settingRepo == nil {
		return config, nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyLoginIPBlockEnabled,
		SettingKeyLoginIPBlockThreshold,
		SettingKeyLoginIPBlockDurationSeconds,
	})
	if err != nil {
		return config, err
	}
	config.Enabled = values[SettingKeyLoginIPBlockEnabled] == "true"
	config.Threshold = parseLoginIPBlockThreshold(values[SettingKeyLoginIPBlockThreshold])
	config.DurationSeconds = parseLoginIPBlockDuration(values[SettingKeyLoginIPBlockDurationSeconds])
	return config, nil
}

func parseLoginIPBlockThreshold(value string) int {
	threshold, err := strconv.Atoi(value)
	if err != nil || threshold < 1 || threshold > 100 {
		return DefaultLoginIPBlockThreshold
	}
	return threshold
}

func parseLoginIPBlockDuration(value string) int {
	duration, err := strconv.Atoi(value)
	if err != nil || !validLoginIPBlockDuration(duration) {
		return DefaultLoginIPBlockDurationSeconds
	}
	return duration
}

func validLoginIPBlockDuration(seconds int) bool {
	_, ok := loginIPBlockDurations[seconds]
	return ok
}
