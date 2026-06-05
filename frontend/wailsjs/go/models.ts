export namespace main {
	
	export class WatcherConfig {
	    id: string;
	    source: string;
	    destination: string;
	    enabled: boolean;
	    wait_time: number;
	    folder_format: string;
	
	    static createFrom(source: any = {}) {
	        return new WatcherConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.source = source["source"];
	        this.destination = source["destination"];
	        this.enabled = source["enabled"];
	        this.wait_time = source["wait_time"];
	        this.folder_format = source["folder_format"];
	    }
	}

}

