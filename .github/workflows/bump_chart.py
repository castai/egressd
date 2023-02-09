import sys
import re

if len(sys.argv) < 2 or sys.argv[1] == '':
    raise 'Chart.yaml path should be passed as first argument'

new_app_version=''
if len(sys.argv) >= 3 and sys.argv[2] != '':
    new_app_version=sys.argv[2]

chart_yaml_path=sys.argv[1]

with open(chart_yaml_path, 'r') as chart_file:
    chart_yaml = chart_file.read()

updated_yaml = chart_yaml

# Auto bump version patch.
match = re.search(r'version:\s*(.+)', chart_yaml)
if match:
    current_version = match.group(1)
    print(f'Current version: {current_version}')

    parts = current_version.split('.')
    current_major = parts[0]
    current_minor = parts[1]
    new_patch = int(parts[2])+1
    new_version = f'{current_major}.{current_minor}.{new_patch}'
    updated_yaml = updated_yaml.replace(current_version, new_version)
    print(f'Updated version: {new_version}')

# Update appVersion.
if new_app_version != '':
    match = re.search(r'appVersion:\s*(.+)', chart_yaml)
    if match:
        current_app_version = match.group(1)
        print(f'Current appVersion: {current_app_version}')

        updated_yaml = updated_yaml.replace(current_app_version, f'"{new_app_version}"')
        print(f'Updated appVersion: "{new_app_version}"')

with open(chart_yaml_path, 'w') as chart_file:
    chart_file.write(updated_yaml)
