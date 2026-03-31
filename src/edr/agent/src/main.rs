use reqwest::Client;
use serde::{Deserialize, Serialize};
use sysinfo::System;
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

#[derive(Deserialize, Debug)]
struct CommandResponse {
    commands: Vec<CommandPayload>,
}

#[derive(Deserialize, Debug)]
struct CommandPayload {
    #[serde(rename = "type")]
    cmd_type: String,
    pid: Option<u32>,
    ip: Option<String>,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    env_logger::init();

    let client = Client::new();
    let mut sys = System::new_all();
    let base_url = std::env::var("BACKEND_URL")
        .unwrap_or_else(|_| "http://localhost:8080".to_string());
    let heartbeat_url = format!("{}/heartbeat", base_url);

    // For MVP, we try to read syslog. In a real app this would be configurable.
    // We'll also support a test file for development: /tmp/nsoc_test.log
    let log_file = if std::path::Path::new("/var/log/syslog").exists() {
        "/var/log/syslog"
    } else {
        println!("[WARN] /var/log/syslog not found. Defaulting to /tmp/nsoc_test.log for testing.");
        "/tmp/nsoc_test.log"
    };

    println!("Agent starting... Target: {}", base_url);

    // Start LogCollector in a separate task
    let collector = LogCollector::new(log_file, client.clone(), &base_url);
    tokio::spawn(async move {
        collector.run().await;
    });

    // Start ProcessMonitor in a separate task
    let monitor = ProcessMonitor::new(client.clone(), &base_url);
    tokio::spawn(async move {
        monitor.run().await;
    });

    // Start Command Polling loop in a separate task
    let cmd_client = client.clone();
    let cmd_base_url = base_url.to_string();
    tokio::spawn(async move {
        let hostname = System::host_name().unwrap_or_else(|| "unknown".to_string());
        let commands_url = format!("{}/commands?hostname={}", cmd_base_url, hostname);
        println!("[CommandPoller] Starting poll loop: {}", commands_url);

        loop {
            sleep(Duration::from_secs(5)).await;

            match cmd_client.get(&commands_url).send().await {
                Ok(resp) => {
                    if resp.status().is_success() {
                        match resp.json::<CommandResponse>().await {
                            Ok(cmd_resp) => {
                                for cmd in cmd_resp.commands {
                                    let executor = CommandExecutor::new();
                                    match cmd.cmd_type.as_str() {
                                        "kill_process" => {
                                            if let Some(pid) = cmd.pid {
                                                println!("[CommandPoller] Executing KillProcess({})", pid);
                                                match executor.execute(AgentCommand::KillProcess(pid)) {
                                                    Ok(_) => println!("[CommandPoller] SUCCESS: Killed process {}", pid),
                                                    Err(e) => eprintln!("[CommandPoller] FAILED to kill process {}: {}", pid, e),
                                                }
                                            } else {
                                                eprintln!("[CommandPoller] kill_process command missing pid");
                                            }
                                        }
                                        "block_ip" => {
                                            if let Some(ip) = cmd.ip {
                                                println!("[CommandPoller] Executing BlockIp({})", ip);
                                                match executor.execute(AgentCommand::BlockIp(ip.clone())) {
                                                    Ok(_) => println!("[CommandPoller] SUCCESS: Blocked IP {}", ip),
                                                    Err(e) => eprintln!("[CommandPoller] FAILED to block IP {}: {}", ip, e),
                                                }
                                            } else {
                                                eprintln!("[CommandPoller] block_ip command missing ip");
                                            }
                                        }
                                        other => {
                                            eprintln!("[CommandPoller] Unknown command type: {}", other);
                                        }
                                    }
                                }
                            }
                            Err(e) => {
                                eprintln!("[CommandPoller] Failed to parse response: {}", e);
                            }
                        }
                    } else {
                        // Don't spam errors if endpoint doesn't exist yet
                        log::debug!("[CommandPoller] Backend returned: {}", resp.status());
                    }
                }
                Err(e) => {
                    log::debug!("[CommandPoller] Failed to poll commands: {}", e);
                }
            }
        }
    });

    // Heartbeat loop
    loop {
        sys.refresh_all();

        let hostname = System::host_name().unwrap_or("unknown".to_string());
        let os = System::name().unwrap_or("unknown".to_string());

        let hb = Heartbeat {
            hostname,
            timestamp: chrono::Utc::now().to_rfc3339(),
            os,
        };

        match client.post(&heartbeat_url).json(&hb).send().await {
            Ok(resp) => {
                if resp.status().is_success() {
                    println!("[SUCCESS] Heartbeat sent to {}", heartbeat_url);
                } else {
                    eprintln!("[ERROR] Backend returned: {}", resp.status());
                }
            }
            Err(e) => eprintln!("[ERROR] Failed to send heartbeat: {}", e),
        }

        sleep(Duration::from_secs(5)).await;
    }

}
