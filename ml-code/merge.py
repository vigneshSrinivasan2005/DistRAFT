import argparse
import os
import json
import torch


def load_model(path):
    return torch.load(path, map_location="cpu")


def average_state_dicts(paths):
    state_dicts = [load_model(p) for p in paths]
    # Initialize with zeros using the first state dict's structure
    avg = {}
    for k in state_dicts[0].keys():
        tensors = [sd[k].float() for sd in state_dicts]
        avg[k] = sum(tensors) / len(tensors)
    return avg


def save_model(state_dict, out_path):
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    torch.save(state_dict, out_path)


def main():
    parser = argparse.ArgumentParser(description="Merge model weights by averaging")
    parser.add_argument("parent_id", type=str, help="Parent job id, e.g. job-1")
    parser.add_argument("--models", nargs="+", required=True, help="List of model file paths to average")
    parser.add_argument("--out", type=str, default=None, help="Output path for merged model")
    args = parser.parse_args()

    out_path = args.out or os.path.join("./raft-data", f"{args.parent_id}_global.pth")

    merged = average_state_dicts(args.models)
    save_model(merged, out_path)

    result = {
        "parent_id": args.parent_id,
        "status": "MERGED",
        "model_path": out_path,
        "num_models": len(args.models),
    }
    print(json.dumps(result))


if __name__ == "__main__":
    main()
