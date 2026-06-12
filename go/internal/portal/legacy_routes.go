package portal

import (
	"net/http"
	"strings"
)

type legacyRouteStrategy string

const (
	legacyStrategyPreserve legacyRouteStrategy = "preserve"
	legacyStrategyProxy    legacyRouteStrategy = "proxy"
	legacyStrategyRedirect legacyRouteStrategy = "redirect"
	legacyStrategyGone     legacyRouteStrategy = "gone"
	legacyStrategyDelete   legacyRouteStrategy = "delete"
)

type legacyRemovalGate string

const (
	legacyRemovalGateUIMigrated       legacyRemovalGate = "ui-migrated"
	legacyRemovalGateClientMigrated   legacyRemovalGate = "client-migrated"
	legacyRemovalGateUsageZero        legacyRemovalGate = "usage-zero"
	legacyRemovalGateExternalContract legacyRemovalGate = "external-contract"
)

type legacyRouteSpec struct {
	LegacyPath      string
	OwnerModule     string
	FormalTarget    string
	CurrentBehavior string
	Strategy        legacyRouteStrategy
	RemovalGate     legacyRemovalGate
}

var portalLegacyRoutes = []legacyRouteSpec{
	{LegacyPath: "/fullsearch/*", OwnerModule: "FullSearchController", FormalTarget: "/articles?mode=full", CurrentBehavior: "legacy result and special detail pages redirect to articles; JSON compatibility endpoints remain during transition", Strategy: legacyStrategyRedirect, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/timelysearch/*", OwnerModule: "TimelySearchController", FormalTarget: "/articles?mode=timely", CurrentBehavior: "legacy timely search pages, data and detail compatibility flow", Strategy: legacyStrategyProxy, RemovalGate: legacyRemovalGateClientMigrated},
	{LegacyPath: "/industry", OwnerModule: "LSearchController", FormalTarget: "/api/v1/search/metadata/types", CurrentBehavior: "legacy industry bucket JSON", Strategy: legacyStrategyProxy, RemovalGate: legacyRemovalGateClientMigrated},
	{LegacyPath: "/getevent", OwnerModule: "LSearchController", FormalTarget: "/api/v1/search/metadata/breadcrumbs", CurrentBehavior: "legacy event bucket JSON", Strategy: legacyStrategyProxy, RemovalGate: legacyRemovalGateClientMigrated},
	{LegacyPath: "/getProvinceList", OwnerModule: "LSearchController", FormalTarget: "/api/v1/search/full/facets", CurrentBehavior: "legacy province list JSON", Strategy: legacyStrategyProxy, RemovalGate: legacyRemovalGateClientMigrated},
	{LegacyPath: "/getArticleCityList", OwnerModule: "LSearchController", FormalTarget: "/api/v1/search/full/facets", CurrentBehavior: "legacy city list JSON", Strategy: legacyStrategyProxy, RemovalGate: legacyRemovalGateClientMigrated},
	{LegacyPath: "/publicoption/reportdetail/*", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}", CurrentBehavior: "legacy detail page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/backanalysis", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=backanalysis", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/eventContext", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=eventContext", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/eventTrace", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=eventTrace", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/hotAnalysis", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=hotAnalysis", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/netizensAnalysis", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=netizensAnalysis", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/statistics", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=statistics", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/propagationAnalysis", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=propagationAnalysis", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/thematicAnalysis", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=thematicAnalysis", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/unscrambleContent", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=unscrambleContent", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/popular_feelings_analys", OwnerModule: "PublicOptionContoller", FormalTarget: "/publicoption?id={id}&section=popular_feelings_analys", CurrentBehavior: "legacy analysis page entry removed after workbench migration", Strategy: legacyStrategyGone, RemovalGate: legacyRemovalGateUIMigrated},
	{LegacyPath: "/publicoption/loadInformation", OwnerModule: "PublicOptionContoller", FormalTarget: "/api/v1/search/full", CurrentBehavior: "legacy article list JSON", Strategy: legacyStrategyProxy, RemovalGate: legacyRemovalGateClientMigrated},
	{LegacyPath: "/platform/nlp/*", OwnerModule: "PlatformController", FormalTarget: "/api/v1/nlp/*", CurrentBehavior: "legacy platform NLP compatibility endpoints", Strategy: legacyStrategyProxy, RemovalGate: legacyRemovalGateExternalContract},
	{LegacyPath: "/platform/xie/*", OwnerModule: "PlatformController", FormalTarget: "/api/v1/nlp/*", CurrentBehavior: "legacy writing tool compatibility and SSE endpoints", Strategy: legacyStrategyProxy, RemovalGate: legacyRemovalGateExternalContract},
	{LegacyPath: "/mobile/*", OwnerModule: "MobileController", FormalTarget: "/portal SSR pages", CurrentBehavior: "legacy mobile compatibility pages", Strategy: legacyStrategyPreserve, RemovalGate: legacyRemovalGateExternalContract},
	{LegacyPath: "/displayboard*", OwnerModule: "DisplayBoardController", FormalTarget: "/portal SSR pages", CurrentBehavior: "legacy dashboard compatibility pages", Strategy: legacyStrategyPreserve, RemovalGate: legacyRemovalGateExternalContract},
	{LegacyPath: "/volume*", OwnerModule: "VolumeController", FormalTarget: "/portal SSR pages", CurrentBehavior: "legacy volume compatibility pages", Strategy: legacyStrategyPreserve, RemovalGate: legacyRemovalGateExternalContract},
	{LegacyPath: "/hot/*", OwnerModule: "HotNewsController", FormalTarget: "/portal SSR pages", CurrentBehavior: "legacy hot news compatibility pages", Strategy: legacyStrategyPreserve, RemovalGate: legacyRemovalGateExternalContract},
	{LegacyPath: "/dist/*", OwnerModule: "UserAuthController", FormalTarget: "/portal SSR pages", CurrentBehavior: "legacy trial application pages", Strategy: legacyStrategyPreserve, RemovalGate: legacyRemovalGateExternalContract},
	{LegacyPath: "/img/code", OwnerModule: "ImageController", FormalTarget: "/login", CurrentBehavior: "legacy captcha endpoint", Strategy: legacyStrategyPreserve, RemovalGate: legacyRemovalGateExternalContract},
	{LegacyPath: "/api/getToken", OwnerModule: "ApiController", FormalTarget: "/api/v1/auth/login", CurrentBehavior: "removed legacy token API", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/api/getArticle", OwnerModule: "ApiController", FormalTarget: "/api/v1/articles", CurrentBehavior: "removed legacy article API", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/api/getMergeArticle", OwnerModule: "ApiController", FormalTarget: "/api/v1/articles", CurrentBehavior: "removed legacy merged article API", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/api/detail", OwnerModule: "ApiController", FormalTarget: "/api/v1/articles/{id}", CurrentBehavior: "removed legacy article detail API", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/monitor/exportarticle", OwnerModule: "MonitorController", FormalTarget: "/articles", CurrentBehavior: "removed legacy export endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/names", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy project names endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/groupandproject", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy project grouping endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/mkdirgroup", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy group create endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/getProjectCountByGroupId", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy group project count endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/editgroup", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy group edit endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/listproject", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy project list endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/getGroupAndProject", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy group and project endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/verifygroup", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy group verify endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/getedit", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy project edit endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/commitproject", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy project commit endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/detail", OwnerModule: "ProjectController", FormalTarget: "/projects/{id}", CurrentBehavior: "removed legacy project detail endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/commiteditproject", OwnerModule: "ProjectController", FormalTarget: "/projects/{id}", CurrentBehavior: "removed legacy project edit submit endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/delProject", OwnerModule: "ProjectController", FormalTarget: "/projects/{id}", CurrentBehavior: "removed legacy project delete endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/updateSolutionGroupStatus", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy project status endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/batchUpdateProject", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy batch update endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/project/keywords", OwnerModule: "ProjectController", FormalTarget: "/projects", CurrentBehavior: "removed legacy project keywords endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/mail/checkMailConfig", OwnerModule: "MailController", FormalTarget: "/system?section=mail", CurrentBehavior: "removed legacy mail config check endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/mail/saveMailConfig", OwnerModule: "MailController", FormalTarget: "/system?section=mail", CurrentBehavior: "removed legacy mail config save endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/mail/getMailConfig", OwnerModule: "MailController", FormalTarget: "/system?section=mail", CurrentBehavior: "removed legacy mail config endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/popUp/needPopUp", OwnerModule: "PopUpController", FormalTarget: "/system?section=popup", CurrentBehavior: "removed legacy popup check endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/popUp/close", OwnerModule: "PopUpController", FormalTarget: "/system?section=popup", CurrentBehavior: "removed legacy popup close endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/popUp/needContact", OwnerModule: "PopUpController", FormalTarget: "/system?section=popup", CurrentBehavior: "removed legacy contact popup endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/popUp/closeContact", OwnerModule: "PopUpController", FormalTarget: "/system?section=popup", CurrentBehavior: "removed legacy contact popup close endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/datamonitor/addfavoritedata", OwnerModule: "DatafavoriteContoller", FormalTarget: "/articles", CurrentBehavior: "removed legacy favorite endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/datamonitor/isread", OwnerModule: "DatafavoriteContoller", FormalTarget: "/articles", CurrentBehavior: "removed legacy read marker endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/datamonitor/selectreadsign", OwnerModule: "DatafavoriteContoller", FormalTarget: "/articles", CurrentBehavior: "removed legacy read query endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/datamonitor/copytext", OwnerModule: "DatafavoriteContoller", FormalTarget: "/articles", CurrentBehavior: "removed legacy copy endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/datamonitor/updateemtion", OwnerModule: "DatafavoriteContoller", FormalTarget: "/articles", CurrentBehavior: "removed legacy emotion endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/datamonitor/sending", OwnerModule: "DatafavoriteContoller", FormalTarget: "/articles", CurrentBehavior: "removed legacy sending endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/datamonitor/deletedata", OwnerModule: "DatafavoriteContoller", FormalTarget: "/articles", CurrentBehavior: "removed legacy delete endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/jumpLogin", OwnerModule: "LoginController", FormalTarget: "/login", CurrentBehavior: "removed legacy login jump endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/wechatJumpLogin", OwnerModule: "LoginController", FormalTarget: "/wechat/checkLogin", CurrentBehavior: "removed legacy wechat jump endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/onlinestatistical", OwnerModule: "LoginController", FormalTarget: "/system?section=account", CurrentBehavior: "removed legacy online status endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/user/save", OwnerModule: "UserController", FormalTarget: "/system?section=account", CurrentBehavior: "removed legacy user save endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
	{LegacyPath: "/user/getToken", OwnerModule: "UserController", FormalTarget: "/api/v1/auth/login", CurrentBehavior: "removed legacy token endpoint", Strategy: legacyStrategyDelete, RemovalGate: legacyRemovalGateUsageZero},
}

func legacyRouteSpecForPath(path string) (legacyRouteSpec, bool) {
	for _, spec := range portalLegacyRoutes {
		if legacyPathMatches(spec.LegacyPath, path) {
			return spec, true
		}
	}
	return legacyRouteSpec{}, false
}

func legacyRouteHasStrategy(path string, strategies ...legacyRouteStrategy) bool {
	spec, ok := legacyRouteSpecForPath(path)
	if !ok {
		return false
	}
	for _, strategy := range strategies {
		if spec.Strategy == strategy {
			return true
		}
	}
	return false
}

func removedLegacyPortalStatus(path string) (int, bool) {
	spec, ok := legacyRouteSpecForPath(path)
	if !ok {
		return 0, false
	}
	switch spec.Strategy {
	case legacyStrategyGone:
		return http.StatusGone, true
	case legacyStrategyDelete:
		return http.StatusNotFound, true
	default:
		return 0, false
	}
}

func legacyPathMatches(pattern, path string) bool {
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	return path == pattern
}
