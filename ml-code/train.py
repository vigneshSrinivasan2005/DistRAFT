#!/usr/bin/env python3
import sys
import os
import json
import argparse
import torch
import torch.nn as nn
import torch.optim as optim
from torch.utils.data import DataLoader, Subset
from torchvision import datasets, transforms

# --- 1. ARGUMENT PARSING (Contract with Go) ---
parser = argparse.ArgumentParser(description='Distributed MNIST Training')
parser.add_argument('job_id', type=str, help='Job ID')
parser.add_argument('--shard_index', type=str, default='node-1', help='Worker shard index (e.g., node-1, node-2)')
parser.add_argument('--total_shards', type=int, default=1, help='Total number of shards (cluster size)')

args = parser.parse_args()
JOB_ID = args.job_id
SHARD_INDEX = args.shard_index
TOTAL_SHARDS = args.total_shards

# Convert node-1, node-2, node-3 to numeric index 0, 1, 2
if SHARD_INDEX.startswith('node-'):
    NUMERIC_SHARD = int(SHARD_INDEX.split('-')[1]) - 1
else:
    NUMERIC_SHARD = 0

# Define where to save this specific job's model
os.makedirs("./raft-data", exist_ok=True)
MODEL_PATH = f"./raft-data/{JOB_ID}_model.pth"

print(f"[Python] 🚀 Starting Training for Job: {JOB_ID}")
print(f"[Python] 📊 Shard {NUMERIC_SHARD + 1}/{TOTAL_SHARDS} (Worker: {SHARD_INDEX})")

class SimpleNN(nn.Module):
    def __init__(self):
        super(SimpleNN, self).__init__()
        self.fc1 = nn.Linear(28 * 28, 128)
        self.fc2 = nn.Linear(128, 64)
        self.fc3 = nn.Linear(64, 10)
        self.relu = nn.ReLU()
        self.dropout = nn.Dropout(0.2)
    
    def forward(self, x):
        x = x.view(-1, 28 * 28)
        x = self.relu(self.fc1(x))
        x = self.dropout(x)
        x = self.relu(self.fc2(x))
        x = self.dropout(x)
        x = self.fc3(x)
        return x

def download_mnist(data_dir="./data"):
    os.makedirs(data_dir, exist_ok=True)
    # Only download if not exists to save bandwidth on re-runs
    train_dataset = datasets.MNIST(
        root=data_dir, train=True, download=True,
        transform=transforms.Compose([
            transforms.ToTensor(),
            transforms.Normalize((0.1307,), (0.3081,))
        ])
    )
    return train_dataset

def train_model(model, train_loader, device, epochs=1):
    criterion = nn.CrossEntropyLoss()
    optimizer = optim.Adam(model.parameters(), lr=0.001)
    model.train()
    
    final_loss = 0.0
    final_acc = 0.0

    for epoch in range(epochs):
        total_loss = 0
        correct = 0
        total = 0
        
        for batch_idx, (data, target) in enumerate(train_loader):
            data, target = data.to(device), target.to(device)
            optimizer.zero_grad()
            outputs = model(data)
            loss = criterion(outputs, target)
            loss.backward()
            optimizer.step()
            
            total_loss += loss.item()
            _, predicted = torch.max(outputs.data, 1)
            total += target.size(0)
            correct += (predicted == target).sum().item()
            
            # Print progress for Go to capture in logs
            if (batch_idx + 1) % 100 == 0:
                print(f"[Python] Job {JOB_ID} - Epoch {epoch+1} - Loss: {loss.item():.4f}")
                sys.stdout.flush() # CRITICAL: Ensure Go sees this immediately
        
        final_loss = total_loss / len(train_loader)
        final_acc = 100 * correct / total

    return final_loss, final_acc

def main():
    sys.stdout.flush()

    try:
        device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        
        # Download full dataset
        full_dataset = download_mnist()
        
        # --- DATA SHARDING LOGIC ---
        total_size = len(full_dataset)  # 60,000 for MNIST train
        chunk_size = total_size // TOTAL_SHARDS
        start = NUMERIC_SHARD * chunk_size
        end = start + chunk_size if NUMERIC_SHARD < TOTAL_SHARDS - 1 else total_size
        
        print(f"[Python] 📦 Dataset shard: {start} to {end} ({end - start} samples)")
        sys.stdout.flush()
        
        # Create subset for this shard
        indices = list(range(start, end))
        train_dataset = Subset(full_dataset, indices)
        train_loader = DataLoader(train_dataset, batch_size=64, shuffle=True)

        model = SimpleNN().to(device)
        
        # Train
        loss, acc = train_model(model, train_loader, device, epochs=1)
        
        # Save Model
        torch.save(model.state_dict(), MODEL_PATH)
        
        # --- 2. JSON OUTPUT (Contract with Go) ---
        # This MUST be the last thing printed
        result = {
            "job_id": JOB_ID,
            "status": "COMPLETED",
            "accuracy": acc,
            "loss": loss,
            "model_path": MODEL_PATH
        }
        print(json.dumps(result))

    except Exception as e:
        # If Python crashes, print a JSON error so Go knows it failed
        error_result = {
            "job_id": JOB_ID,
            "status": "FAILED",
            "error": str(e)
        }
        print(json.dumps(error_result))
        sys.exit(1)

if __name__ == "__main__":
    main()