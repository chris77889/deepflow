/*
 * Copyright (c) 2024 Yunshan Networks
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package vtap

import (
	"fmt"
	"strings"
	"sync"

	. "github.com/deepflowio/deepflow/server/controller/common"
	. "github.com/deepflowio/deepflow/server/controller/trisolaris/common"
	metadata "github.com/deepflowio/deepflow/server/controller/trisolaris/metadata/agentmetadata"
	. "github.com/deepflowio/deepflow/server/controller/trisolaris/utils"
)

type AgentPlatformData struct {

	// 下的云平台列表=xxx，容器集群内部IP下发=所有集群
	// key为vtap_group_lcuuid
	platformDataType1 *AgentPlatformDataType

	// 下发的云平台列表=全部，容器集群内部IP下发=采集器所在集群
	// key为vtap_group_lcuuid+采集器所在容器集群LCUUID
	platformDataType2 *AgentPlatformDataType

	// 下发的云平台列表=xxx，容器集群内部IP下发=采集器所在集群
	// key为vtap_group_lcuuid+集器所在容器集群LCUUID
	platformDataType3 *AgentPlatformDataType

	// 专属采集器
	platformDataBMDedicated *AgentPlatformDataType

	ORGID
}

func newAgentPlatformData(orgID int) *AgentPlatformData {
	return &AgentPlatformData{
		platformDataType1:       newAgentPlatformDataType("platformDataType1"),
		platformDataType2:       newAgentPlatformDataType("platformDataType2"),
		platformDataType3:       newAgentPlatformDataType("platformDataType3"),
		platformDataBMDedicated: newAgentPlatformDataType("platformDataBMDedicated"),
		ORGID:                   ORGID(orgID),
	}
}

func (v *AgentPlatformData) String() string {
	log.Debug(v.Logf("%s", v.platformDataType1))
	log.Debug(v.Logf("%s", v.platformDataType2))
	log.Debug(v.Logf("%s", v.platformDataType3))
	log.Debug(v.Logf("%s", v.platformDataBMDedicated))
	return "agent platform data"
}

type AgentPlatformDataType struct {
	sync.RWMutex
	platformDataMap map[string]*metadata.PlatformData
	name            string
}

func newAgentPlatformDataType(name string) *AgentPlatformDataType {
	return &AgentPlatformDataType{
		platformDataMap: make(map[string]*metadata.PlatformData),
		name:            name,
	}
}

func (t *AgentPlatformDataType) String() string {
	t.RLock()
	defer t.RUnlock()
	for k, v := range t.platformDataMap {
		log.Debug("key: [%s]; value:[%s]", k, v)
	}
	return t.name
}

func (t *AgentPlatformDataType) setPlatformDataCache(key string, data *metadata.PlatformData) {
	t.Lock()
	defer t.Unlock()
	t.platformDataMap[key] = data
}

func (t *AgentPlatformDataType) getPlatformDataCache(key string) *metadata.PlatformData {
	t.RLock()
	defer t.RUnlock()
	return t.platformDataMap[key]
}

func (t *AgentPlatformDataType) clearCache() {
	t.Lock()
	defer t.Unlock()
	t.platformDataMap = make(map[string]*metadata.PlatformData)
}

func (v *AgentPlatformData) clearAgentPlatformDataTypeCache() {
	v.platformDataType1.clearCache()
	v.platformDataType2.clearCache()
	v.platformDataType3.clearCache()
	v.platformDataBMDedicated.clearCache()
}

// TODO: add userConfig
func (v *AgentPlatformData) setPlatformDataByAgent(p *metadata.PlatformDataOP, c *VTapCache) {
	vTapType := c.GetVTapType()
	// 隧道解封装采集器没有平台数据
	if vTapType == VTAP_TYPE_TUNNEL_DECAPSULATION {
		return
	}

	log.Debug(v.Logf("set platfrom data to %s %s %s", c.GetCtrlIP(), c.GetCtrlMac(), c.getPodDomains()))
	vTapGroupLcuuid := c.GetVTapGroupLcuuid()
	vtapConfig := c.GetVTapConfig()
	if vtapConfig == nil {
		return
	}

	podClusterInternalIP := vtapConfig.getPodClusterInternalIP()
	domainFilters := vtapConfig.getDomainFilters()
	log.Debug(v.Logf("%d %s", podClusterInternalIP, domainFilters))
	if !podClusterInternalIP && SliceEqual[string](domainFilters, ALL_DOMAIMS) {
		// 下发的云平台列表=全部，容器集群内部IP下发=所有集群
		// 所有云平台所有数据
		log.Info(v.Logf("all: %s", p.GetAllSimplePlatformData()))
		c.setAgentPlatformData(p.GetAllSimplePlatformData())
	} else if !podClusterInternalIP {
		// 下发的云平台列表=xxx，容器集群内部IP下发=所有集群
		// 云平台列表=xxx的所有数据

		// 获取缓存数据
		data := v.platformDataType1.getPlatformDataCache(vTapGroupLcuuid)
		if data != nil {
			c.setAgentPlatformData(data)
			return
		}
		domainToAllPlatformData := p.GetDomainToAllPlatformData()
		domainAllData := metadata.NewPlatformData("platformDataType1", "", 0, PLATFORM_DATA_TYPE_1)
		for _, domainLcuuid := range domainFilters {
			domainData := domainToAllPlatformData[domainLcuuid]
			if domainData == nil {
				log.Errorf(v.Logf("domain(%s) no platform data", domainLcuuid))
				continue
			}
			domainAllData.Merge(domainData)
		}
		domainAllData.MergePeerConnProtos(p.GetNoDomainPlatformData())
		domainAllData.GeneratePlatformDataResult()
		v.platformDataType1.setPlatformDataCache(vTapGroupLcuuid, domainAllData)
		c.setAgentPlatformData(domainAllData)
		log.Debug(v.Logf("%s", domainAllData))
	} else if podClusterInternalIP && SliceEqual[string](domainFilters, ALL_DOMAIMS) {
		// 下发的云平台列表=全部，容器集群内部IP下发=采集器所在集群
		// 所有云平台中devicetype != POD/容器服务的所有接口，采集器所在集群devicetype=POD/容器服务的所有接口

		// 专属服务器类型：所有集群
		if vTapType == VTAP_TYPE_DEDICATED {
			data := p.GetAllSimplePlatformData()
			c.setAgentPlatformData(data)
			log.Debug(v.Logf("vtap_type_dedicated: %s", data))
			return
		}
		// 获取缓存数据
		podDomains := c.getPodDomains()
		key := fmt.Sprintf("%s+%s", vTapGroupLcuuid, strings.Join(podDomains, "+"))
		data := v.platformDataType2.getPlatformDataCache(key)
		if data != nil {
			c.setAgentPlatformData(data)
			return
		}
		domainToPlarformDataOnlyPod := p.GetDomainToPlatformDataOnlyPod()
		domainAllData := metadata.NewPlatformData("platformDataType2", "", 0, PLATFORM_DATA_TYPE_2)
		domainAllData.Merge(p.GetAllSimplePlatformDataExceptPod())
		for _, podDomain := range podDomains {
			vTapDomainData := domainToPlarformDataOnlyPod[podDomain]
			if vTapDomainData == nil {
				log.Errorf(v.Logf("vtap pod domain(%s) no data", podDomain))
				continue
			}
			domainAllData.MergeInterfaces(vTapDomainData)
		}
		domainAllData.GeneratePlatformDataResult()
		c.setAgentPlatformData(domainAllData)
		v.platformDataType2.setPlatformDataCache(key, domainAllData)
		log.Debug(v.Logf("%s", domainAllData))
	} else if podClusterInternalIP {
		// 下发的云平台列表=xxx，容器集群内部IP下发=采集器所在集群
		// 云平台列表=xxx中devicetype != POD/容器服务所有接口，集器所在集群devicetype=POD/容器服务的所有接口

		// 专属服务器类型：下发的云平台列表=xxx，容器集群内部IP下发=所有集群
		if vTapType == VTAP_TYPE_DEDICATED {
			// 获取缓存数据
			data := v.platformDataBMDedicated.getPlatformDataCache(vTapGroupLcuuid)
			if data != nil {
				c.setAgentPlatformData(data)
				return
			}
			domainToAllPlatformData := p.GetDomainToAllPlatformData()
			domainAllData := metadata.NewPlatformData("platformDataBMDedicated", "", 0, PLATFORM_DATA_BM_DEDICATED)
			for _, domainLcuuid := range vtapConfig.ConvertedDomains {
				domainData := domainToAllPlatformData[domainLcuuid]
				if domainData == nil {
					log.Errorf(v.Logf("domain(%s) no platform data", domainLcuuid))
					continue
				}
				domainAllData.Merge(domainData)
			}
			domainAllData.MergePeerConnProtos(p.GetNoDomainPlatformData())
			domainAllData.GeneratePlatformDataResult()
			c.setAgentPlatformData(domainAllData)
			v.platformDataBMDedicated.setPlatformDataCache(vTapGroupLcuuid, domainAllData)
			log.Debug(v.Logf("%s", domainAllData))
			return
		}

		// 获取缓存数据
		podDomains := c.getPodDomains()
		key := fmt.Sprintf("%s+%s", vTapGroupLcuuid, strings.Join(podDomains, "+"))
		data := v.platformDataType3.getPlatformDataCache(key)
		if data != nil {
			c.setAgentPlatformData(data)
			return
		}

		domainToPlatformDataExceptPod := p.GetDomainToPlatformDataExceptPod()
		domainToPlarformDataOnlyPod := p.GetDomainToPlatformDataOnlyPod()
		domainAllData := metadata.NewPlatformData("platformDataType3", "", 0, PLATFORM_DATA_TYPE_3)
		for _, domainLcuuid := range domainFilters {
			domainData := domainToPlatformDataExceptPod[domainLcuuid]
			if domainData == nil {
				log.Errorf(v.Logf("domain(%s) no platform data", domainLcuuid))
				continue
			}
			domainAllData.Merge(domainData)
		}

		for _, podDomain := range podDomains {
			vtapDomainData := domainToPlarformDataOnlyPod[podDomain]
			if vtapDomainData == nil {
				log.Errorf(v.Logf("domain(%s) no platform data", podDomain))
				continue
			}
			if Find[string](domainFilters, podDomain) {
				domainAllData.MergeInterfaces(vtapDomainData)
			} else {
				domainAllData.Merge(vtapDomainData)
			}
		}

		domainAllData.MergePeerConnProtos(p.GetNoDomainPlatformData())
		domainAllData.GeneratePlatformDataResult()
		c.setAgentPlatformData(domainAllData)
		v.platformDataType3.setPlatformDataCache(key, domainAllData)
		log.Debug(v.Logf("%s", domainAllData))
	}
}
