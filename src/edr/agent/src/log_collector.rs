use std::fs::File;
use std::io::{BufRead, BufReader, Seek, SeekFrom};
use std::time::Duration;
use tokio::time::sleep;
use reqwest::Client;
use serde::Serialize;
use sysinfo::System;

#[derive(Serialize, Debug)]
pub struct LogEntry {
    pub hostname: String,
    pub log: String,
    pub timestamp: String,
}

pub struct LogCollector {
    file_path: String,
    client: Client,
    base_url: String,
}

impl LogCollector {
    pub fn new(file_path: &str, client: Client, base_url: &str) -> Self {
        Self {
            file_path: file_path.to_string(),
            client,
            base_url: base_url.to_string(),
        }
    }

    pub async fn run(&self) {
        println!("[LogCollector] Starting watch on: {}", self.file_path);

        let mut file = match File::open(&self.file_path) {
            Ok(f) => f,
            Err(e) => {
                eprintln!("[LogCollector] Error opening file {}: {}", self.file_path, e);
                return;
            }
        };

        // Move to the end of the file to read only new logs
        if let Err(e) = file.seek(SeekFrom::End(0)) {
            eprintln!("[LogCollector] Error seeking file: {}", e);
            return;
        }

        let mut reader = BufReader::new(file);
        let mut line = String::new();
        let logs_url = format!("{}/logs", self.base_url);

        loop {
            line.clear();
            match reader.read_line(&mut line) {
                Ok(0) => {
                    // EOF, wait a bit
                    sleep(Duration::from_millis(500)).await;
                }
                Ok(_) => {
                    let trimmed = line.trim();
                    if !trimmed.is_empty() {
                        println!("[LogCollector] NEW LOG: {}", trimmed);

                        let entry = LogEntry {
                            hostname: System::host_name().unwrap_or_else(|| "unknown".to_string()),
                            log: trimmed.to_string(),
                            timestamp: chrono::Utc::now().to_rfc3339(),
                        };

                        // Fire-and-forget POST to backend
                        let client = self.client.clone();
                        let url = logs_url.clone();
                        tokio::spawn(async move {
                            match client.post(&url).json(&entry).send().await {
                                Ok(resp) => {
                                    if !resp.status().is_success() {
                                        eprintln!("[LogCollector] Backend returned: {}", resp.status());
                                    }
                                }
                                Err(e) => {
                                    eprintln!("[LogCollector] Failed to POST log: {}", e);
                                }
                            }
                        });
                    }
                }
                Err(e) => {
                    eprintln!("[LogCollector] Error reading line: {}", e);
                    sleep(Duration::from_secs(1)).await;
                }
            }
        }
    }
}
