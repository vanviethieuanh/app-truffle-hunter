# Extract Owner/Repo from CSV and get distinct lines without sorting
tail -n +2 "$1" | awk -F, '{print $2}' | uniq > repos.txt

# Use GNU Parallel to scan repositories
parallel -a repos.txt ./scan_repo.sh
