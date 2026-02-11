use reqwest::Client;
use serde::{Deserialize, Serialize};
use sysinfo::System;
use std::process::Command;
use std::time::Duration;
use tokio::time::sleep;

mod log_collector;
use log_collector::LogCollector;

mod process_monitor;
use process_monitor::ProcessMonitor;

mod command_executor;
use command_executor::{AgentCommand, CommandExecutor};

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
    // For MVP, we try to read syslog. In a real app this would be configurable.
    // We'll also support a test file for development: /tmp/nsoc_test.log
    let log_file = if std::path::Path::new("/var/log/syslog").exists() {
        "/var/log/syslog"
    } else {
        println!("[WARN] /var/log/syslog not found. Defaulting to /tmp/nsoc_test.log for testing.");
        "/tmp/nsoc_test.log"
    };

    println!("Agent starting... Target: {}", backend_url);

    // Start LogCollector in a separate task
    let collector = LogCollector::new(log_file);
    tokio::spawn(async move {
        collector.run().await;
    });

    // Start ProcessMonitor in a separate task
    let monitor = ProcessMonitor::new();
    tokio::spawn(async move {
        monitor.run().await;
    });

    // SIMULATION: Test CommandExecutor
    tokio::spawn(async {
        sleep(Duration::from_secs(5)).await;
        println!("\n[SIMULATION] Starting CommandExecutor test...");
        
        // 1. Spawn a dummy process
        let child = Command::new("sleep")
            .arg("100")
            .spawn();
            
        match child {
            Ok(child) => {
                let pid = child.id();
                println!("[SIMULATION] Spawned dummy process with PID: {}", pid);
                sleep(Duration::from_secs(2)).await;
                
                let executor = CommandExecutor::new();
                match executor.execute(AgentCommand::KillProcess(pid)) {
                    Ok(_) => println!("[SIMULATION] SUCCESS: Killed process {}", pid),
                    Err(e) => eprintln!("[SIMULATION] FAILED to kill process: {}", e),
                }
                
                // 2. Test IP Block (likely to fail without root, but good to verify invocation)
                sleep(Duration::from_secs(1)).await;
                println!("[SIMULATION] Testing IP Block (expect failure if not root)...");
                match executor.execute(AgentCommand::BlockIp("192.168.1.100".to_string())) {
                    Ok(_) => println!("[SIMULATION] SUCCESS: Blocked IP"),
                    Err(e) => eprintln!("[SIMULATION] Expected failure (permission/path): {}", e),
                }
            }
            Err(e) => eprintln!("[SIMULATION] Failed to spawn dummy process: {}", e),
        }
    });

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
