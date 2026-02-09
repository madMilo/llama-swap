package proxy

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

type UINavigationItem struct {
	Label  string
	Path   string
	Active bool
}

type UIVersionInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

type UIModel struct {
	ID          string
	Name        string
	Description string
	Source      string
	Aliases     []string
}

type UIRunningProcess struct {
	Model string
	Name  string
	State string
	Proxy string
	TTL   string
}

type UIPageData struct {
	NavItems         []UINavigationItem
	VersionInfo      UIVersionInfo
	Models           []UIModel
	RunningProcesses []UIRunningProcess
	Logs             string
}

func (pm *ProxyManager) uiIndexHandler(c *gin.Context) {
	c.Redirect(http.StatusFound, "/ui/models")
}

func (pm *ProxyManager) uiModelsPageHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/models")
	data.Models = pm.uiModelsList()
	pm.renderUITemplate(c, "pages/models", data)
}

func (pm *ProxyManager) uiRunningPageHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/running")
	data.RunningProcesses = pm.uiRunningList()
	pm.renderUITemplate(c, "pages/running", data)
}

func (pm *ProxyManager) uiLogsPageHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/logs")
	data.Logs = string(pm.muxLogger.GetHistory())
	pm.renderUITemplate(c, "pages/logs", data)
}

func (pm *ProxyManager) uiModelsPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/models")
	data.Models = pm.uiModelsList()
	pm.renderUITemplate(c, "partials/models", data)
}

func (pm *ProxyManager) uiRunningPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/running")
	data.RunningProcesses = pm.uiRunningList()
	pm.renderUITemplate(c, "partials/running", data)
}

func (pm *ProxyManager) uiLogsPartialHandler(c *gin.Context) {
	data := pm.uiPageData("/ui/logs")
	data.Logs = string(pm.muxLogger.GetHistory())
	pm.renderUITemplate(c, "partials/logs", data)
}

func (pm *ProxyManager) uiPageData(activePath string) UIPageData {
	return UIPageData{
		NavItems: []UINavigationItem{
			{Label: "Models", Path: "/ui/models", Active: activePath == "/ui/models"},
			{Label: "Running", Path: "/ui/running", Active: activePath == "/ui/running"},
			{Label: "Logs", Path: "/ui/logs", Active: activePath == "/ui/logs"},
		},
		VersionInfo: UIVersionInfo{
			Version:   pm.version,
			Commit:    pm.commit,
			BuildDate: pm.buildDate,
		},
	}
}

func (pm *ProxyManager) uiModelsList() []UIModel {
	models := make([]UIModel, 0, len(pm.config.Models))
	for id, modelConfig := range pm.config.Models {
		if modelConfig.Unlisted {
			continue
		}
		aliases := []string{}
		if pm.config.IncludeAliasesInList {
			for _, alias := range modelConfig.Aliases {
				alias = strings.TrimSpace(alias)
				if alias != "" {
					aliases = append(aliases, alias)
				}
			}
		}
		models = append(models, UIModel{
			ID:          id,
			Name:        strings.TrimSpace(modelConfig.Name),
			Description: strings.TrimSpace(modelConfig.Description),
			Source:      "local",
			Aliases:     aliases,
		})
	}

	if pm.peerProxy != nil {
		for peerID, peer := range pm.peerProxy.ListPeers() {
			for _, modelID := range peer.Models {
				models = append(models, UIModel{
					ID:     modelID,
					Name:   fmt.Sprintf("%s: %s", peerID, modelID),
					Source: fmt.Sprintf("peer:%s", peerID),
				})
			}
		}
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	return models
}

func (pm *ProxyManager) uiRunningList() []UIRunningProcess {
	processes := make([]UIRunningProcess, 0)
	for _, processGroup := range pm.processGroups {
		for _, process := range processGroup.processes {
			if process.CurrentState() != StateReady {
				continue
			}
			ttl := ""
			if process.config.UnloadAfter > 0 {
				ttl = fmt.Sprintf("%ds", process.config.UnloadAfter)
			}
			processes = append(processes, UIRunningProcess{
				Model: process.ID,
				Name:  strings.TrimSpace(process.config.Name),
				State: string(process.CurrentState()),
				Proxy: strings.TrimSpace(process.config.Proxy),
				TTL:   ttl,
			})
		}
	}

	sort.Slice(processes, func(i, j int) bool {
		return processes[i].Model < processes[j].Model
	})

	return processes
}

func (pm *ProxyManager) renderUITemplate(c *gin.Context, name string, data UIPageData) {
	if pm.uiTemplates == nil {
		c.String(http.StatusInternalServerError, "UI templates unavailable")
		return
	}
	tmpl := pm.uiTemplates.Template(name)
	if tmpl == nil {
		c.String(http.StatusInternalServerError, "UI template not found")
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(c.Writer, data); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
	}
}
