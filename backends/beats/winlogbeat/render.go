// This file is part of Graylog.
//
// Graylog is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

package winlogbeat

import (
	"bytes"
	"github.com/Graylog2/collector-sidecar/api/graylog"
	"github.com/Graylog2/collector-sidecar/common"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os/exec"
	"text/template"
)

func (wlbc *WinLogBeatConfig) snippetsToString() string {
	var buffer bytes.Buffer
	var result bytes.Buffer
	for _, snippet := range wlbc.Beats.Snippets {
		snippetTemplate, err := template.New("snippet").Parse(snippet.Value)
		if err != nil {
			result.WriteString(snippet.Value)
		} else {
			snippetTemplate.Execute(&buffer, wlbc.Beats.Context.Inventory)
			result.WriteString(buffer.String())
		}
		result.WriteString("\n")
	}
	return result.String()
}

func (wlbc *WinLogBeatConfig) Render() bytes.Buffer {
	var result bytes.Buffer

	if wlbc.Beats.Data() == nil {
		return result
	}

	result.WriteString(wlbc.Beats.String())
	result.WriteString(wlbc.snippetsToString())

	return result
}

func (wlbc *WinLogBeatConfig) RenderToFile() error {
	stringConfig := wlbc.Render()
	err := common.CreatePathToFile(wlbc.Beats.UserConfig.ConfigurationPath)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(wlbc.Beats.UserConfig.ConfigurationPath, stringConfig.Bytes(), 0644)
	return err
}

func (wlbc *WinLogBeatConfig) RenderOnChange(response graylog.ResponseCollectorConfiguration) bool {
	newConfig := NewCollectorConfig(wlbc.Beats.Context)

	// create prospector slice
	var eventlogs []map[string]interface{}

	for _, output := range response.Outputs {
		if output.Backend == "winlogbeat" {
			for property, value := range output.Properties {
				newConfig.Beats.Set(value, "output", output.Type, property)
			}
		}
	}

	for _, input := range response.Inputs {
		if input.Backend == "winlogbeat" {
			eventlogs = append(eventlogs, make(map[string]interface{}))
			idx := len(eventlogs) - 1
			for property, value := range input.Properties {
				var vt interface{}
				err := yaml.Unmarshal([]byte(value), &vt)
				if err != nil {
					log.Errorf("[%s] Nested YAML is not parsable: '%s'", wlbc.Name(), value)
				} else {
					eventlogs[idx][property] = vt
				}
			}
		}
	}
	newConfig.Beats.Set(eventlogs, "winlogbeat", "event_logs")

	for _, snippet := range response.Snippets {
		if snippet.Backend == "winlogbeat" {
			newConfig.Beats.AppendString(snippet.Id, snippet.Value)
		}
	}

	if !wlbc.Beats.Equals(newConfig.Beats) {
		log.Infof("[%s] Configuration change detected, rewriting configuration file.", wlbc.Name())
		wlbc.Beats.Update(newConfig.Beats)
		wlbc.RenderToFile()
		return true
	}

	return false
}

func (wlbc *WinLogBeatConfig) ValidateConfigurationFile() bool {
	cmd := exec.Command(wlbc.ExecPath(), "-configtest", "-c", wlbc.Beats.UserConfig.ConfigurationPath)
	err := cmd.Run()
	if err != nil {
		log.Errorf("[%s] Error during configuration validation: %s", wlbc.Name(), err)
		return false
	}

	return true
}