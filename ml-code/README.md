# ML Training Code

This directory contains Python training scripts that run alongside the Go RAFT consensus layer.

## Setup

Install dependencies:
```bash
pip install -r requirements.txt
```

## Running MNIST Training

```bash
python train.py
```

This will:
1. **Download** the MNIST dataset (only on first run, cached locally)
2. **Build** a simple 3-layer neural network (784 → 128 → 64 → 10)
3. **Train** for 1 epoch (60,000 samples, batch size 64)
4. **Save** the trained model to `model.pth`

Output:
- Model weights saved to `./model.pth` (~300 KB)
- Dataset cached in `./data/` directory
- Training logs show loss and accuracy per epoch

## Architecture

The neural network consists of:
- **Input layer**: 784 neurons (28×28 MNIST images)
- **Hidden layer 1**: 128 neurons + ReLU + Dropout(0.2)
- **Hidden layer 2**: 64 neurons + ReLU + Dropout(0.2)
- **Output layer**: 10 neurons (digit classes 0-9)

Optimizer: Adam (lr=0.001)
Loss: Cross-Entropy
Expected accuracy after 1 epoch: ~95%

## Integration with Go

Future versions will use gRPC to:
- Stream model parameters from the Go leader to Python workers
- Stream computed gradients back to the Go node for aggregation

See `../internal/api/ml_service.proto` for the gRPC contract.
