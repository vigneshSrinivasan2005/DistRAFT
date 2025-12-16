#!/usr/bin/env python3
import sys
import os
import json
import torch
import torch.nn as nn
import torch.optim as optim
from torch.utils.data import DataLoader
from torchvision import datasets, transforms

# --- 1. ARGUMENT PARSING (Contract with Go) ---
if len(sys.argv) < 2:
    # Fallback for manual testing
    print("Warning: No Job ID provided. Using 'manual-test'.")
    JOB_ID = "manual-test"
else:
    JOB_ID = sys.argv[1]

# Define where to save this specific job's model
MODEL_PATH = f"./raft-data/{JOB_ID}_model.pth"

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
    # Force print to flush immediately so Go logs are real-time
    print(f"[Python] 🚀 Starting Training for Job: {JOB_ID}")
    sys.stdout.flush()

    try:
        device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        
        # Use a subset of data for speed in this demo
        full_dataset = download_mnist()
        subset_indices = torch.arange(2000) # Only train on 2000 images for speed
        train_dataset = torch.utils.data.Subset(full_dataset, subset_indices)
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