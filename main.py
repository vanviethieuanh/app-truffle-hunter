#!/usr/bin/env python3

import asyncio
import json
import logging
import logging.config
import os
from asyncio import Semaphore, TaskGroup, subprocess
from typing import Any, Generator, List

import aiofiles
from dotenv import load_dotenv
from github import Github

# Limit the number of concurrent scans
sem = Semaphore(3)


async def scan(obj: str, result_file: str):
    async with sem:
        try:
            logging.info(f"Starting hog for org: {obj}")

            cmd = [
                "trufflehog",
                "github",
                "--no-update",
                "--json",
                f"--org={obj}",
            ]
            process = await subprocess.create_subprocess_exec(
                *cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE
            )

            logging.info(f"Started hog: {process.pid}")

            # Read the output of the process
            stdout, stderr = await process.communicate()

            if stderr:
                logging.error(f"Error in trufflehog process: {stderr.decode()}")

            # Write the output to the result file
            async with aiofiles.open(result_file, mode="a") as f:
                if stdout:
                    await f.write(
                        stdout.decode("utf-8") + "\n"
                    )  # Decode and append new line

            await process.wait()
        except Exception as e:
            logging.error(f"Failed to execute scan for {obj}: {e}")


async def main():
    # Load logging configuration from log.conf
    logging.config.fileConfig("log.conf")

    # Load environment variables from the .env file
    load_dotenv(override=True)

    ACCESS_TOKEN = os.getenv("ACCESS_TOKEN")
    if not ACCESS_TOKEN:
        logging.error("ACCESS_TOKEN not found in environment variables")
        exit(1)

    # Initialize GitHub object
    logging.info("Initializing GitHub client")
    gh_client = Github(login_or_token=ACCESS_TOKEN)

    with open("search_queries.json", "r") as queries_file:
        queries = json.load(queries_file)

    result_file = "result.ndjson"

    # Search
    search_results = search(gh_client, queries)
    scanned_target = set()

    scan_tasks = []
    async with TaskGroup() as tg:
        for repo_owner, repo_name, file_path, file_url in search_results:
            if repo_owner in scanned_target:
                logging.info(f"{repo_owner} scanned, skipping...")
                continue
            else:
                logging.info(
                    f"Add {repo_owner} to scanned list, starting scan..."
                )
                scanned_target.add(repo_owner)
                # Schedule the scan task
                tg.create_task(scan(repo_owner, result_file))
                logging.info(
                    f"Scan task queued"
                )
                await asyncio.sleep(0)


def search(
    gh_client: Github, queries: List[str]
) -> Generator[tuple[str, str, str, str], Any, None]:
    for query in queries:
        try:
            logging.info(f"Performing search query: {query}")
            result = gh_client.search_code(query.get("query"))
        except Exception as e:
            logging.error(f"Failed to execute search query: {e}")
            exit(1)

        for file in result:
            repo_name = file.repository.full_name
            repo_owner = file.repository.owner.login  # Get the owner/org
            file_path = file.path
            file_url = file.html_url

            logging.info(
                f"Repository: {repo_name}, Owner: {repo_owner}, File: {file_path}, URL: {file_url}"
            )

            # Write results to CSV
            yield (repo_owner, repo_name, file_path, file_url)


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logging.info("Keyboard Interrupted!")
