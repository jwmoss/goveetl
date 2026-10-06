package govee

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const automationPath = "/bff-app/v1/ecology/resource/automation/details"

type AutomationView struct {
	ID        int                    `json:"id"`
	Name      string                 `json:"name"`
	Enabled   bool                   `json:"enabled"`
	Actions   []AutomationActionView `json:"actions"`
	groupSort *int
}

type AutomationActionView struct {
	Device       string `json:"device"`
	Sku          string `json:"sku"`
	Name         string `json:"name"`
	Power        *int   `json:"power,omitempty"`
	Brightness   *int   `json:"brightness,omitempty"`
	TemperatureK *int   `json:"temperature_k,omitempty"`
	Scene        string `json:"scene,omitempty"`
}

type AutomationLightSettings struct {
	Power        *int
	Brightness   *int
	TemperatureK *int
}

func (a *App) ListAutomations(ctx context.Context) ([]AutomationView, error) {
	data, err := a.Do(ctx, "GET", "/bff-app/v1/ecology/resource/automations", nil, nil)
	if err != nil {
		return nil, err
	}
	var env Envelope[struct {
		AutoExecs []struct {
			GroupID   int    `json:"groupId"`
			Name      string `json:"name"`
			Enable    int    `json:"enable"`
			GroupSort *int   `json:"groupSort"`
		} `json:"autoExecs"`
	}]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if err := env.Err(); err != nil {
		return nil, err
	}
	views := make([]AutomationView, 0, len(env.Data.AutoExecs))
	for _, item := range env.Data.AutoExecs {
		views = append(views, AutomationView{ID: item.GroupID, Name: item.Name, Enabled: item.Enable == 1, Actions: []AutomationActionView{}, groupSort: item.GroupSort})
	}
	return views, nil
}

func (a *App) ReadAutomation(ctx context.Context, id int) (*AutomationView, error) {
	doc, err := a.automation(ctx, id)
	if err != nil {
		return nil, err
	}
	return automationView(doc)
}

func (a *App) automation(ctx context.Context, id int) (map[string]any, error) {
	if id <= 0 {
		return nil, fmt.Errorf("automation ID must be positive")
	}
	data, err := a.Do(ctx, "GET", automationPath, map[string][]string{"groupId": {strconv.Itoa(id)}}, nil)
	if err != nil {
		return nil, err
	}
	var env Envelope[json.RawMessage]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if err := env.Err(); err != nil {
		return nil, err
	}
	var doc map[string]any
	decoder := json.NewDecoder(bytes.NewReader(env.Data))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	if number(doc["groupId"]) != id {
		return nil, fmt.Errorf("automation response has the wrong ID")
	}
	return doc, nil
}

func automationActions(doc map[string]any) ([]map[string]any, error) {
	linkage, _ := doc["linkage"].(map[string]any)
	groups, _ := linkage["ruleGroups"].([]any)
	actions := []map[string]any{}
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid automation action group")
		}
		rules, ok := group["iotRules"].([]any)
		if !ok {
			return nil, fmt.Errorf("invalid automation actions")
		}
		for _, item := range rules {
			action, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid automation device action")
			}
			if _, ok := action["deviceObj"].(map[string]any); !ok {
				return nil, fmt.Errorf("invalid automation device")
			}
			actions = append(actions, action)
		}
	}
	return actions, nil
}

func automationView(doc map[string]any) (*AutomationView, error) {
	view := &AutomationView{ID: number(doc["groupId"]), Name: textValue(doc["name"]), Enabled: number(doc["enable"]) == 1, Actions: []AutomationActionView{}}
	actions, err := automationActions(doc)
	if err != nil {
		return nil, err
	}
	for _, action := range actions {
		device := action["deviceObj"].(map[string]any)
		item := AutomationActionView{Device: textValue(device["device"]), Sku: textValue(device["sku"]), Name: textValue(device["name"])}
		rules, _ := action["rule"].([]any)
		for _, entry := range rules {
			rule, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid automation command")
			}
			var value map[string]json.RawMessage
			if err := json.Unmarshal([]byte(textValue(rule["cmdVal"])), &value); err != nil {
				continue
			}
			switch number(rule["cmdType"]) {
			case 0:
				_ = json.Unmarshal(value["open"], &item.Power)
			case 1:
				_ = json.Unmarshal(value["brightness"], &item.Brightness)
			case 3:
				_ = json.Unmarshal(value["name"], &item.Scene)
			case 5:
				_ = json.Unmarshal(value["colorTempKelvin"], &item.TemperatureK)
			}
		}
		view.Actions = append(view.Actions, item)
	}
	return view, nil
}

func (a *App) RemoveAutomationDevice(ctx context.Context, id int, device, sku string) (*AutomationView, error) {
	doc, err := a.automation(ctx, id)
	if err != nil {
		return nil, err
	}
	actions, err := automationActions(doc)
	if err != nil {
		return nil, err
	}
	if len(actions) == 0 {
		return nil, fmt.Errorf("automation has no supported device actions")
	}
	linkage := doc["linkage"].(map[string]any)
	removed := 0
	for _, item := range linkage["ruleGroups"].([]any) {
		group := item.(map[string]any)
		kept := []any{}
		for _, entry := range group["iotRules"].([]any) {
			action := entry.(map[string]any)
			if automationDeviceMatches(action, device, sku) {
				removed++
				continue
			}
			kept = append(kept, entry)
		}
		if len(kept) == 0 {
			return nil, fmt.Errorf("cannot remove the last device from an action group")
		}
		group["iotRules"] = kept
	}
	if removed == 0 {
		return nil, fmt.Errorf("device not in automation")
	}
	return a.saveAutomation(ctx, doc)
}

func (a *App) SetAutomationLight(ctx context.Context, id int, device, sku string, settings AutomationLightSettings) (*AutomationView, error) {
	if settings.Power == nil && settings.Brightness == nil && settings.TemperatureK == nil {
		return nil, fmt.Errorf("choose power, brightness, or temperature")
	}
	if settings.Power != nil && *settings.Power != 0 && *settings.Power != 1 {
		return nil, fmt.Errorf("power must be on or off")
	}
	if settings.Brightness != nil && (*settings.Brightness < 1 || *settings.Brightness > 100) {
		return nil, fmt.Errorf("brightness must be 1-100")
	}
	// ponytail: support the known H706C 2700 K wire command; add mappings after device verification.
	if settings.TemperatureK != nil && (sku != "H706C" || *settings.TemperatureK != 2700) {
		return nil, fmt.Errorf("automation temperature currently supports H706C at 2700 K")
	}
	doc, err := a.automation(ctx, id)
	if err != nil {
		return nil, err
	}
	actions, err := automationActions(doc)
	if err != nil {
		return nil, err
	}
	var target map[string]any
	for _, action := range actions {
		if automationDeviceMatches(action, device, sku) {
			if target != nil {
				return nil, fmt.Errorf("device appears in multiple action groups; select an automation with one device action")
			}
			target = action
		}
	}
	if target == nil {
		return nil, fmt.Errorf("device not in automation")
	}
	if number(target["cmdGroup"]) != 1 {
		return nil, fmt.Errorf("unsupported automation command group")
	}
	rules, ok := target["rule"].([]any)
	if !ok || len(rules) == 0 {
		return nil, fmt.Errorf("automation device has no command template")
	}
	type commandTemplate struct {
		Msg struct {
			AccountTopic string
			CmdVersion   int
		}
	}
	var template commandTemplate
	versions := map[int]int{}
	for _, entry := range rules {
		rule, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid automation command")
		}
		var candidate commandTemplate
		if json.Unmarshal([]byte(textValue(rule["iotMsg"])), &candidate) == nil && candidate.Msg.AccountTopic != "" {
			if template.Msg.AccountTopic == "" {
				template = candidate
			}
			versions[number(rule["cmdType"])] = candidate.Msg.CmdVersion
		}
	}
	if template.Msg.AccountTopic == "" {
		return nil, fmt.Errorf("automation command template has no account topic")
	}
	kept := []any{}
	for _, entry := range rules {
		rule := entry.(map[string]any)
		kind := number(rule["cmdType"])
		if (settings.Power != nil && kind == 0) || (settings.Brightness != nil && kind == 1) {
			continue
		}
		if settings.TemperatureK != nil {
			if kind == 2 || kind == 3 || kind == 5 {
				continue
			}
			if kind != 0 && kind != 1 {
				return nil, fmt.Errorf("cannot replace unsupported light-effect command %d", kind)
			}
		}
		kept = append(kept, entry)
	}
	add := func(kind int, command string, value, data any) error {
		version, ok := versions[kind]
		if kind != 5 && !ok {
			return fmt.Errorf("saved %s command has no supported MQTT template", command)
		}
		// H706C native colorwc uses version zero; other commands retain their saved version.
		if kind == 5 {
			version = 0
		}
		wire, err := WriteEnvelope("", template.Msg.AccountTopic, command, version, data)
		if err != nil {
			return err
		}
		wire["msg"].(map[string]any)["origin"] = 35
		valueJSON, err := json.Marshal(value)
		if err != nil {
			return err
		}
		wireJSON, err := json.Marshal(wire)
		if err != nil {
			return err
		}
		kept = append(kept, map[string]any{"cmdType": kind, "cmdVal": string(valueJSON), "iotMsg": string(wireJSON)})
		return nil
	}
	if settings.Power != nil {
		if err := add(0, "turn", map[string]int{"open": *settings.Power}, map[string]int{"val": *settings.Power}); err != nil {
			return nil, err
		}
	}
	if settings.Brightness != nil {
		if err := add(1, "brightness", map[string]int{"brightness": *settings.Brightness}, map[string]int{"val": *settings.Brightness}); err != nil {
			return nil, err
		}
	}
	if settings.TemperatureK != nil {
		// Constant.java maps 2700 K to ARGB ffffae54; ColorTemObj carries both values.
		value := map[string]any{"colorTempValue": -20908, "colorTempKelvin": 2700, "timeStamp": time.Now().UnixMilli()}
		data := map[string]any{"color": map[string]int{"r": 0, "g": 0, "b": 0}, "colorTemInKelvin": 2700}
		if err := add(5, "colorwc", value, data); err != nil {
			return nil, err
		}
	}
	target["rule"] = kept
	return a.saveAutomation(ctx, doc)
}

func (a *App) saveAutomation(ctx context.Context, doc map[string]any) (*AutomationView, error) {
	// Details omit the list order; preserve it from the catalog instead of writing the app's default zero.
	if _, ok := doc["groupSort"]; !ok {
		items, err := a.ListAutomations(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.ID == number(doc["groupId"]) && item.groupSort != nil {
				doc["groupSort"] = *item.groupSort
				break
			}
		}
		if _, ok := doc["groupSort"]; !ok {
			return nil, fmt.Errorf("automation catalog has no saved list order")
		}
	}
	// AutoExecuteEditActivity.F4 clears the inactive schedule before saving linkage actions.
	if schedule, ok := doc["schedule"].(map[string]any); ok && len(schedule) == 0 {
		delete(doc, "schedule")
	}
	data, err := a.Do(ctx, "PUT", automationPath, nil, doc)
	if err != nil {
		return nil, err
	}
	var env Envelope[json.RawMessage]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if err := env.Err(); err != nil {
		return nil, err
	}
	after, err := a.automation(ctx, number(doc["groupId"]))
	if err != nil {
		return nil, fmt.Errorf("automation write accepted but verification failed: %w", err)
	}
	if schedule, ok := after["schedule"].(map[string]any); ok && len(schedule) == 0 {
		delete(after, "schedule")
	}
	for _, key := range []string{"name", "enable", "cmdType", "schedule", "triggerRule", "linkage"} {
		// Compare JSON to normalize numbers introduced by the command builder.
		x, _ := json.Marshal(after[key])
		y, _ := json.Marshal(doc[key])
		if !bytes.Equal(x, y) {
			return nil, fmt.Errorf("automation write accepted but verification failed for %s", key)
		}
	}
	return automationView(after)
}

func automationDeviceMatches(action map[string]any, device, sku string) bool {
	obj := action["deviceObj"].(map[string]any)
	return textValue(obj["device"]) == device && textValue(obj["sku"]) == sku
}

func number(v any) int       { n, _ := strconv.Atoi(fmt.Sprint(v)); return n }
func textValue(v any) string { s, _ := v.(string); return s }
