use reqwest::Client;
use serde::{Deserialize, Serialize};
use sysinfo::System;
use std::time::Duration;
use tokio::time::sleep;

#[derive(Serialize, Deserialize, Debug)]
struct Heartbeat {
    hostname: String,
    timestamp: String,
    os: String,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    env_logger::init();
    
    let client = Client::new();
    let mut sys = System::new_all();
    let backend_url = "http://localhost:8080/heartbeat";

    println!("Agent starting... Target: {}", backend_url);

    loop {
        sys.refresh_all();
        
        let hostname = System::host_name().unwrap_or("unknown".to_string());
        let os = System::name().unwrap_or("unknown".to_string());
        
        let hb = Heartbeat {
            hostname,
            timestamp: chrono::Utc::now().to_rfc3339(),
            os,
        };

        match client.post(backend_url).json(&hb).send().await {
            Ok(resp) => {
                if resp.status().is_success() {
                    println!("[SUCCESS] Heartbeat sent to {}", backend_url);
                } else {
                    eprintln!("[ERROR] Backend returned: {}", resp.status());
                }
            }
            Err(e) => eprintln!("[ERROR] Failed to send heartbeat: {}", e),
        }

        sleep(Duration::from_secs(5)).await;
    }

}
