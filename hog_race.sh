# Extract Owner/Repo from CSV
tail -n +2 search_results.csv | awk -F, '{print $2}' >repos.txt

# Use GNU Parallel to scan repositories
parallel -a repos.txt ./scan_repo.sh
