package config

import (
	"fmt"
	"sort"
	"strings"
)

func expandModelDefinitions(config Config) (Config, error) {
	if len(config.ModelSources) == 0 || len(config.ParameterSets) == 0 {
		return config, nil
	}

	if config.Models == nil {
		config.Models = make(map[string]ModelConfig)
	}

	sourceIDs := make([]string, 0, len(config.ModelSources))
	for id := range config.ModelSources {
		sourceIDs = append(sourceIDs, id)
	}
	sort.Strings(sourceIDs)

	paramIDs := make([]string, 0, len(config.ParameterSets))
	for id := range config.ParameterSets {
		paramIDs = append(paramIDs, id)
	}
	sort.Strings(paramIDs)

	for _, sourceID := range sourceIDs {
		source := config.ModelSources[sourceID]
		for _, paramID := range paramIDs {
			param := config.ParameterSets[paramID]
			modelID := fmt.Sprintf("%s:%s", sourceID, paramID)

			generated, err := buildGeneratedModelConfig(sourceID, paramID, source, param)
			if err != nil {
				return Config{}, err
			}

			if override, exists := config.Models[modelID]; exists {
				generated = mergeModelConfig(generated, override)
			}

			config.Models[modelID] = generated
		}
	}

	return config, nil
}

func buildGeneratedModelConfig(sourceID, paramID string, source ModelSourceConfig, param ParameterSetConfig) (ModelConfig, error) {
	model := DefaultModelConfig()

	model.Cmd = strings.TrimSpace(source.Cmd)
	if source.Path != "" {
		model.Macros = append(model.Macros, MacroEntry{Name: "MODEL_PATH", Value: source.Path})
	}

	model.Macros = mergeMacroLists(model.Macros, source.Macros)
	model.Macros = mergeMacroLists(model.Macros, param.Macros)

	model.CmdStop = firstNonEmpty(source.CmdStop, model.CmdStop)
	model.Proxy = firstNonEmpty(source.Proxy, model.Proxy)
	model.CheckEndpoint = firstNonEmpty(source.CheckEndpoint, model.CheckEndpoint)
	model.UseModelName = firstNonEmpty(param.UseModelName, source.UseModelName)

	model.Name = firstNonEmpty(param.Name, source.Name)
	model.Description = firstNonEmpty(param.Description, source.Description)

	model.Env = append(model.Env, source.Env...)
	model.Env = append(model.Env, param.Env...)

	model.Aliases = append(model.Aliases, source.Aliases...)
	model.Aliases = append(model.Aliases, param.Aliases...)

	model.Filters = mergeModelFilters(source.Filters, param.Filters)
	model.Metadata = mergeMetadata(source.Metadata, param.Metadata)

	if source.UnloadAfter > 0 {
		model.UnloadAfter = source.UnloadAfter
	}
	if param.UnloadAfter > 0 {
		model.UnloadAfter = param.UnloadAfter
	}

	if source.Unlisted {
		model.Unlisted = true
	}
	if param.Unlisted {
		model.Unlisted = true
	}

	if source.VramMB > 0 {
		model.VramMB = source.VramMB
	}
	if param.VramMB > 0 {
		model.VramMB = param.VramMB
	}

	if source.MinVramMB > 0 {
		model.MinVramMB = source.MinVramMB
	}
	if param.MinVramMB > 0 {
		model.MinVramMB = param.MinVramMB
	}

	if source.FitPolicy != "" {
		model.FitPolicy = source.FitPolicy
	}
	if param.FitPolicy != "" {
		model.FitPolicy = param.FitPolicy
	}

	if source.CpuMoe > 0 {
		model.CpuMoe = source.CpuMoe
	}
	if param.CpuMoe > 0 {
		model.CpuMoe = param.CpuMoe
	}

	if source.ConcurrencyLimit > 0 {
		model.ConcurrencyLimit = source.ConcurrencyLimit
	}
	if param.ConcurrencyLimit > 0 {
		model.ConcurrencyLimit = param.ConcurrencyLimit
	}

	if source.SendLoadingState != nil {
		model.SendLoadingState = source.SendLoadingState
	}
	if param.SendLoadingState != nil {
		model.SendLoadingState = param.SendLoadingState
	}

	if model.Cmd == "" {
		return ModelConfig{}, fmt.Errorf("modelSources.%s: cmd is required", sourceID)
	}

	if param.Args != "" {
		if model.Cmd == "" {
			model.Cmd = strings.TrimSpace(param.Args)
		} else {
			model.Cmd = strings.TrimSpace(model.Cmd) + "\n" + strings.TrimSpace(param.Args)
		}
	}

	model.Macros = mergeMacroLists(model.Macros, MacroList{
		{Name: "MODEL_SOURCE", Value: sourceID},
		{Name: "PARAM_SET", Value: paramID},
	})

	return model, nil
}

func mergeModelFilters(base ModelFilters, overlay ModelFilters) ModelFilters {
	merged := base
	if overlay.StripParams != "" {
		merged.StripParams = overlay.StripParams
	}
	if len(overlay.SetParams) > 0 {
		merged.SetParams = overlay.SetParams
	}
	return merged
}

func mergeMetadata(base, overlay map[string]any) map[string]any {
	if base == nil && overlay == nil {
		return nil
	}
	merged := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func mergeModelConfig(base, override ModelConfig) ModelConfig {
	merged := base

	if override.Cmd != "" {
		merged.Cmd = override.Cmd
	}
	if override.CmdStop != "" {
		merged.CmdStop = override.CmdStop
	}
	if override.Proxy != "" {
		merged.Proxy = override.Proxy
	}
	if override.CheckEndpoint != "" {
		merged.CheckEndpoint = override.CheckEndpoint
	}
	if override.UseModelName != "" {
		merged.UseModelName = override.UseModelName
	}
	if override.Name != "" {
		merged.Name = override.Name
	}
	if override.Description != "" {
		merged.Description = override.Description
	}
	if len(override.Env) > 0 {
		merged.Env = override.Env
	}
	if len(override.Aliases) > 0 {
		merged.Aliases = override.Aliases
	}
	if override.UnloadAfter > 0 {
		merged.UnloadAfter = override.UnloadAfter
	}
	if override.Unlisted {
		merged.Unlisted = override.Unlisted
	}
	if override.VramMB > 0 {
		merged.VramMB = override.VramMB
	}
	if override.MinVramMB > 0 {
		merged.MinVramMB = override.MinVramMB
	}
	if override.FitPolicy != "" {
		merged.FitPolicy = override.FitPolicy
	}
	if override.CpuMoe > 0 {
		merged.CpuMoe = override.CpuMoe
	}
	if override.ConcurrencyLimit > 0 {
		merged.ConcurrencyLimit = override.ConcurrencyLimit
	}
	if override.SendLoadingState != nil {
		merged.SendLoadingState = override.SendLoadingState
	}
	if len(override.Macros) > 0 {
		merged.Macros = override.Macros
	}
	if len(override.Metadata) > 0 {
		merged.Metadata = override.Metadata
	}
	if override.Filters.StripParams != "" || len(override.Filters.SetParams) > 0 {
		merged.Filters = override.Filters
	}

	return merged
}

func mergeMacroLists(base MacroList, overlay MacroList) MacroList {
	if len(overlay) == 0 {
		return base
	}
	merged := make(MacroList, 0, len(base)+len(overlay))
	merged = append(merged, base...)
	for _, entry := range overlay {
		replaced := false
		for i, existing := range merged {
			if existing.Name == entry.Name {
				merged[i] = entry
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, entry)
		}
	}
	return merged
}

func firstNonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
