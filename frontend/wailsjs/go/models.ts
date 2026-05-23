export namespace catalogue {
	
	export class RbxmProps {
	    dump_label?: string;
	    instance_path?: string;
	    class_name?: string;
	    prop?: string;
	
	    static createFrom(source: any = {}) {
	        return new RbxmProps(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dump_label = source["dump_label"];
	        this.instance_path = source["instance_path"];
	        this.class_name = source["class_name"];
	        this.prop = source["prop"];
	    }
	}
	export class CatalogueEntry {
	    label: string;
	    character: any;
	    model?: string;
	    role: string;
	    asset_type: string;
	    slot?: number;
	    target_id: number;
	    notes?: string;
	    rbxm_props?: RbxmProps;
	
	    static createFrom(source: any = {}) {
	        return new CatalogueEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.character = source["character"];
	        this.model = source["model"];
	        this.role = source["role"];
	        this.asset_type = source["asset_type"];
	        this.slot = source["slot"];
	        this.target_id = source["target_id"];
	        this.notes = source["notes"];
	        this.rbxm_props = this.convertValues(source["rbxm_props"], RbxmProps);
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
	
	export class Snapshot {
	    format_version: number;
	    name: string;
	    description: string;
	    replacements_catalogue: CatalogueEntry[];
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format_version = source["format_version"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.replacements_catalogue = this.convertValues(source["replacements_catalogue"], CatalogueEntry);
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
	
	export class CertPatchStatus {
	    total: number;
	    patched: number;
	    unpatched: number;
	    details: string[];
	
	    static createFrom(source: any = {}) {
	        return new CertPatchStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.patched = source["patched"];
	        this.unpatched = source["unpatched"];
	        this.details = source["details"];
	    }
	}
	export class InstalledPackInfo {
	    profile_id: string;
	    name: string;
	    author: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new InstalledPackInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile_id = source["profile_id"];
	        this.name = source["name"];
	        this.author = source["author"];
	        this.version = source["version"];
	    }
	}
	export class ProxyStatus {
	    active: boolean;
	    elevated: boolean;
	    lastLifecycle?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.elevated = source["elevated"];
	        this.lastLifecycle = source["lastLifecycle"];
	    }
	}

}

export namespace pack {
	
	export class Replacement {
	    label: string;
	    character: any;
	    role: string;
	    asset_type: string;
	    slot?: number;
	    target_id: number;
	    replace_with?: number;
	    replace_with_file?: string;
	
	    static createFrom(source: any = {}) {
	        return new Replacement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.character = source["character"];
	        this.role = source["role"];
	        this.asset_type = source["asset_type"];
	        this.slot = source["slot"];
	        this.target_id = source["target_id"];
	        this.replace_with = source["replace_with"];
	        this.replace_with_file = source["replace_with_file"];
	    }
	}
	export class Pack {
	    format_version: number;
	    name: string;
	    author: string;
	    version: string;
	    description: string;
	    replacements: Replacement[];
	
	    static createFrom(source: any = {}) {
	        return new Pack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format_version = source["format_version"];
	        this.name = source["name"];
	        this.author = source["author"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.replacements = this.convertValues(source["replacements"], Replacement);
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

