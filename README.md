# Prefix Fetcher

**Prefix Fetcher** is a Go-based tool for fetching country-specific IP prefixes from BGP data. Currently supports Iranian (IR), Chinese (CN), and Russian (RU) IP prefixes with easy extensibility for additional countries.

## Features

- Fetches IP prefixes from [bgp.tools](https://bgp.tools/table.jsonl)
- Filters prefixes by country-specific ASN numbers
- Converts IPv4 prefixes to /24 blocks for efficient processing
- Supports multiple countries (IR, CN, RU) with clean separation
- Concurrent processing for fast execution
- Automatic retry logic with exponential backoff

## Prerequisites

- **Go (>=1.24)**

## Build

1. Clone the repository:

    ```sh
    git clone https://github.com/compassvpn/prefix-fetcher.git
    cd prefix-fetcher
    ```

2. Build the application:

    ```sh
    go build
    ```

## Usage

```sh
./prefix-fetcher [OPTIONS]
```

### Available Options

| Option        | Short | Description                                    |
|---------------|-------|------------------------------------------------|
| `--fetch-ir`  |       | Fetch Iranian IP prefixes from bgp.tools      |
| `--fetch-cn`  |       | Fetch Chinese IP prefixes from bgp.tools      |
| `--fetch-ru`  |       | Fetch Russian IP prefixes from bgp.tools      |
| `--verbose`   | `-v`  | Enable verbose logging                         |
| `--version`   |       | Show version information                       |

### Examples

- Fetch Iranian IP prefixes:

  ```sh
  ./prefix-fetcher --fetch-ir
  ```

- Fetch Chinese IP prefixes:

  ```sh
  ./prefix-fetcher --fetch-cn
  ```

- Fetch Russian IP prefixes:

  ```sh
  ./prefix-fetcher --fetch-ru
  ```

- Fetch with verbose output:

  ```sh
  ./prefix-fetcher --fetch-ir --verbose
  ```

## Output Files

The tool generates country-specific prefix files:

**Iranian prefixes:**
- `ir_prefixes_v4.txt` - IPv4 prefixes as /24 blocks
- `ir_prefixes_v6.txt` - IPv6 prefixes

**Chinese prefixes:**
- `cn_prefixes_v4.txt` - IPv4 prefixes as /24 blocks  
- `cn_prefixes_v6.txt` - IPv6 prefixes

**Russian prefixes:**
- `ru_prefixes_v4.txt` - IPv4 prefixes as /24 blocks
- `ru_prefixes_v6.txt` - IPv6 prefixes

## How It Works

1. **Downloads BGP Data:** Fetches the complete BGP routing table from bgp.tools
2. **Filters by ASN:** Keeps only prefixes from country-specific Autonomous System Numbers
3. **Processes IPv4:** Converts IPv4 prefixes to /24 blocks for consistency
4. **Sorts and Saves:** Outputs clean, sorted prefix lists to text files

## License

This project is licensed under the MIT License.

## Contributions

Contributions are welcome! Feel free to fork the repository and submit a pull request.
