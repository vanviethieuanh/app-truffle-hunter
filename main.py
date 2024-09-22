import csv
import json
import logging
import logging.config
import os

from dotenv import load_dotenv
from github import Github


def search(gh_client: Github, query: str, csv_writer):
    try:
        logging.info(f"Performing search query: {query}")
        result = gh_client.search_code(query)
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
        csv_writer.writerow([repo_owner, repo_name, file_path, file_url])


def main():
    # Load logging configuration from log.conf
    logging.config.fileConfig("log.conf")

    # Load environment variables from the .env file
    load_dotenv()

    ACCESS_TOKEN = os.getenv("ACCESS_TOKEN")
    if not ACCESS_TOKEN:
        logging.error("ACCESS_TOKEN not found in environment variables")
        exit(1)

    # Initialize GitHub object
    logging.info("Initializing GitHub client")
    gh_client = Github(ACCESS_TOKEN)

    # Open the CSV file in write mode
    with open('search_results.csv', mode='a', newline='', encoding='utf-8') as file:
        csv_writer = csv.writer(file)
        # Write the header row
        csv_writer.writerow(['Owner/Org', 'Repository', 'File Path', 'URL'])

        # Load search queries from JSON file
        with open('search_queries.json', 'r') as queries_file:
            queries = json.load(queries_file)

        # Perform searches for each query
        for query_data in queries:
            query = query_data['query']
            logging.info(f"Starting search for query: {query}")
            search(gh_client, query, csv_writer)

    logging.info("Search completed and results written to CSV")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        logging.info("Keyboard Interrupted!")
        logging.info("Keyboard Interrupted!")
