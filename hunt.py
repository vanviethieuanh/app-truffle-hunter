import argparse
import asyncio
import os
import subprocess
from typing import Set


def scan(obj: str):
    cmd = [
        "trufflehog",
        "git",
        "--no-update",
        "--json",
        f"https://github.com/${obj}",
        ">> result.txt",
    ]

    process = subprocess.Popen(cmd)
    return process


if __name__ == "__main__":
    with open('search_result.csv', )


    for e in entrypoints:
        scan(e)