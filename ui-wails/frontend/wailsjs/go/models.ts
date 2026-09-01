export namespace config {
	
	export class PartySettings {
	    enabled: boolean;
	    serverUrl: string;
	    clientId: string;
	    displayName: string;
	    serverPort?: number;
	    partyCode?: string;
	
	    static createFrom(source: any = {}) {
	        return new PartySettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.serverUrl = source["serverUrl"];
	        this.clientId = source["clientId"];
	        this.displayName = source["displayName"];
	        this.serverPort = source["serverPort"];
	        this.partyCode = source["partyCode"];
	    }
	}
	export class UpdateSettings {
	    enableAutoCheck: boolean;
	    checkIntervalHours: number;
	    updateChannel: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enableAutoCheck = source["enableAutoCheck"];
	        this.checkIntervalHours = source["checkIntervalHours"];
	        this.updateChannel = source["updateChannel"];
	    }
	}
	export class ReconnectOptions {
	    reconnectIntervalMs: number;
	    maxReconnectAttempts: number;
	
	    static createFrom(source: any = {}) {
	        return new ReconnectOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reconnectIntervalMs = source["reconnectIntervalMs"];
	        this.maxReconnectAttempts = source["maxReconnectAttempts"];
	    }
	}
	export class MonitorOptions {
	    debounceTimeMs: number;
	    ignoreInitial: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MonitorOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.debounceTimeMs = source["debounceTimeMs"];
	        this.ignoreInitial = source["ignoreInitial"];
	    }
	}
	export class Config {
	    remoteId: string;
	    screenshotDir: string;
	    logsDir: string;
	    autoConnect: boolean;
	    autoProcessExisting: boolean;
	    positionUpdateThrottleMs: number;
	    debugLogging: boolean;
	    darkMode: boolean;
	    enableQuestTracking: boolean;
	    tarkovTrackerUrl: string;
	    tarkovTrackerToken: string;
	    tarkovTrackerGameMode: string;
	    monitorOptions: MonitorOptions;
	    reconnectOptions: ReconnectOptions;
	    updateSettings: UpdateSettings;
	    partySettings: PartySettings;
	    setupComplete: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.remoteId = source["remoteId"];
	        this.screenshotDir = source["screenshotDir"];
	        this.logsDir = source["logsDir"];
	        this.autoConnect = source["autoConnect"];
	        this.autoProcessExisting = source["autoProcessExisting"];
	        this.positionUpdateThrottleMs = source["positionUpdateThrottleMs"];
	        this.debugLogging = source["debugLogging"];
	        this.darkMode = source["darkMode"];
	        this.enableQuestTracking = source["enableQuestTracking"];
	        this.tarkovTrackerUrl = source["tarkovTrackerUrl"];
	        this.tarkovTrackerToken = source["tarkovTrackerToken"];
	        this.tarkovTrackerGameMode = source["tarkovTrackerGameMode"];
	        this.monitorOptions = this.convertValues(source["monitorOptions"], MonitorOptions);
	        this.reconnectOptions = this.convertValues(source["reconnectOptions"], ReconnectOptions);
	        this.updateSettings = this.convertValues(source["updateSettings"], UpdateSettings);
	        this.partySettings = this.convertValues(source["partySettings"], PartySettings);
	        this.setupComplete = source["setupComplete"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	

}

export namespace main {
	
	export class LogEntry {
	    timestamp: string;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.level = source["level"];
	        this.message = source["message"];
	    }
	}

}

export namespace updater {
	
	export class UpdateInfo {
	    version: string;
	    releaseUrl: string;
	    // Go type: time
	    releaseDate: any;
	    releaseName: string;
	    releaseBody: string;
	    assetUrl: string;
	    assetName: string;
	    assetSize: number;
	    isPrerelease: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.releaseUrl = source["releaseUrl"];
	        this.releaseDate = this.convertValues(source["releaseDate"], null);
	        this.releaseName = source["releaseName"];
	        this.releaseBody = source["releaseBody"];
	        this.assetUrl = source["assetUrl"];
	        this.assetName = source["assetName"];
	        this.assetSize = source["assetSize"];
	        this.isPrerelease = source["isPrerelease"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

