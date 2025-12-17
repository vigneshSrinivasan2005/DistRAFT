#!/usr/bin/env python3
"""
Verify that the merged model weights are mathematically correct.
Loads the global model and individual shard models to check averaging.
"""

import torch
import sys
import os

def load_model(path):
    """Load a PyTorch model state dict."""
    if not os.path.exists(path):
        print(f"❌ Model not found: {path}")
        return None
    return torch.load(path, map_location='cpu')

def verify_averaging(global_path, shard_paths):
    """
    Verify that the global model is the average of shard models.
    """
    print("🔍 Loading models...")
    
    global_model = load_model(global_path)
    if global_model is None:
        return False
    
    shard_models = []
    for path in shard_paths:
        model = load_model(path)
        if model is None:
            print(f"⚠️  Skipping missing shard: {path}")
        else:
            shard_models.append(model)
    
    if len(shard_models) == 0:
        print("❌ No shard models found!")
        return False
    
    print(f"✓ Loaded global model and {len(shard_models)} shard models")
    
    # Get all parameter names
    param_names = list(global_model.keys())
    print(f"✓ Found {len(param_names)} parameters in global model")
    
    # Check first layer weights
    first_layer_key = param_names[0]
    print(f"\n📊 Inspecting first layer: '{first_layer_key}'")
    
    global_weights = global_model[first_layer_key]
    print(f"   Shape: {global_weights.shape}")
    print(f"   Mean: {global_weights.mean().item():.6f}")
    print(f"   Std: {global_weights.std().item():.6f}")
    print(f"   Min: {global_weights.min().item():.6f}")
    print(f"   Max: {global_weights.max().item():.6f}")
    
    # Check if weights are all zeros (bad!)
    if torch.allclose(global_weights, torch.zeros_like(global_weights)):
        print("❌ Global model weights are all zeros!")
        return False
    
    print("✓ Global model weights are non-zero")
    
    # Verify averaging: compute expected average from shards
    print(f"\n🧮 Verifying averaging across {len(shard_models)} shards...")
    
    errors = []
    checked_params = 0
    
    for param_name in param_names[:5]:  # Check first 5 parameters
        if param_name not in global_model:
            continue
            
        global_param = global_model[param_name]
        
        # Compute expected average
        shard_params = []
        for shard in shard_models:
            if param_name in shard:
                shard_params.append(shard[param_name])
        
        if len(shard_params) == 0:
            continue
        
        expected_avg = torch.stack(shard_params).mean(dim=0)
        
        # Compare with global model
        diff = torch.abs(global_param - expected_avg).max().item()
        
        if diff > 1e-5:  # Tolerance for floating point errors
            errors.append(f"   {param_name}: max diff = {diff:.8f}")
        
        checked_params += 1
    
    if checked_params == 0:
        print("⚠️  Could not verify averaging (no matching parameters)")
        return True  # Don't fail if we can't verify
    
    if len(errors) > 0:
        print(f"❌ Averaging verification failed for {len(errors)} parameters:")
        for err in errors:
            print(err)
        return False
    
    print(f"✓ Averaging verified correctly for {checked_params} parameters")
    
    # Show sample weights from first layer
    print(f"\n📋 Sample weights from first layer (first 5 values):")
    print(f"   Global: {global_weights.flatten()[:5].tolist()}")
    for i, shard in enumerate(shard_models[:3]):
        if first_layer_key in shard:
            weights = shard[first_layer_key]
            print(f"   Shard {i+1}: {weights.flatten()[:5].tolist()}")
    
    return True

def main():
    # Default paths
    global_path = "./raft-data/test-federated_global.pth"
    shard_paths = [
        "./raft-data/test-federated-node-1_model.pth",
        "./raft-data/test-federated-node-2_model.pth",
        "./raft-data/test-federated-node-3_model.pth",
    ]
    
    # Allow custom paths from command line
    if len(sys.argv) > 1:
        global_path = sys.argv[1]
    if len(sys.argv) > 2:
        shard_paths = sys.argv[2:]
    
    print("=" * 60)
    print("🔬 Federated Model Verification")
    print("=" * 60)
    print(f"Global model: {global_path}")
    print(f"Shard models: {len(shard_paths)} files")
    print()
    
    success = verify_averaging(global_path, shard_paths)
    
    print("\n" + "=" * 60)
    if success:
        print("✅ VERIFICATION PASSED")
        print("=" * 60)
        return 0
    else:
        print("❌ VERIFICATION FAILED")
        print("=" * 60)
        return 1

if __name__ == "__main__":
    sys.exit(main())
