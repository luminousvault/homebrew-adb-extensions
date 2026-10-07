#!/bin/bash
#@@BUILD_EXCLUDE_START
# ═══════════════════════════════════════════════════
# MAC Command
# 디바이스 Wi-Fi / Bluetooth MAC 주소 조회
# ═══════════════════════════════════════════════════
#@@BUILD_EXCLUDE_END

# Completion definition: command name and description
: <<'AK_COMPLETION_DESC'
mac:Show device MAC addresses
AK_COMPLETION_DESC

# Completion handler: zsh completion code for mac command
: <<'AK_COMPLETION'
        mac)
          _arguments \
            '(- *)'{-h,--help}'[Show help for this command]' \
            '-m[Show all connected devices]'
          ;;
AK_COMPLETION

show_help_mac() {
    echo -e "${CYAN}${BOLD}Usage:${NC} ak mac [-m] [-h|--help]"
    echo
    echo "Description: Show MAC addresses of the device."
    echo "  Wi-Fi (current)  MAC used for the current connection (per-network, may be randomized)"
    echo "  Wi-Fi (factory)  Device's own Wi-Fi MAC (available even when Wi-Fi is off, if exposed)"
    echo "  Bluetooth        Bluetooth MAC"
    echo
    echo "Options:"
    echo "  -m          Show all connected devices"
    echo "  -h, --help  Show this help message"
    echo
    exit 1
}

# ─────────────────────────────────────────────────────
# MAC 조회 헬퍼
# ─────────────────────────────────────────────────────

# MAC 값을 소문자로 정규화. 형식이 아니거나 무효값(02:00..., 00:00...)이면 실패
normalize_mac() {
    local mac
    mac=$(echo "$1" | head -1 | tr -d '\r' | tr 'A-F' 'a-f')
    [[ "$mac" =~ ^([0-9a-f]{2}:){5}[0-9a-f]{2}$ ]] || return 1
    [[ "$mac" == "02:00:00:00:00:00" || "$mac" == "00:00:00:00:00:00" ]] && return 1
    echo "$mac"
}

# 디바이스 셸 명령 출력에서 "<prefix><MAC>" 형태의 첫 값을 추출
# filter가 있으면 해당 문자열을 포함한 줄에서만 찾음
# 성공 시 G_WIFI_MAC, G_WIFI_STATUS(원본 출력, SSID 추출용) 설정
try_wifi_source() {
    local serial="$1" shell_cmd="$2" prefix="$3" filter="$4"
    local out mac
    out=$(adb -s "$serial" shell "$shell_cmd 2>/dev/null" 2>/dev/null | tr -d '\r')
    [ -n "$filter" ] && out=$(echo "$out" | grep -F "$filter")
    mac=$(normalize_mac "$(echo "$out" | grep -o "${prefix}[0-9a-fA-F:]*" | head -1 | awk '{print $NF}')") || return 1
    G_WIFI_MAC="$mac"
    G_WIFI_STATUS="$out"
    debug_log "Wi-Fi MAC from '$shell_cmd': $mac"
}

# 현재 연결에 사용 중인 Wi-Fi MAC 폴백 체인. 결과: G_WIFI_MAC (미연결 시 빈 값), G_WIFI_STATUS
# Wi-Fi가 꺼져도 WifiInfo에 마지막 MAC이 남으므로 연결 완료(COMPLETED) 상태의 줄만 사용
get_wifi_mac() {
    local serial="$1" api
    G_WIFI_MAC=""
    G_WIFI_STATUS=""
    api=$(adb -s "$serial" shell getprop ro.build.version.sdk 2>/dev/null | tr -d '\r')

    if [[ "$api" =~ ^[0-9]+$ ]] && [ "$api" -ge 30 ] && try_wifi_source "$serial" "cmd wifi status" "MAC: " "Supplicant state: COMPLETED"; then
        return 0
    fi
    try_wifi_source "$serial" "dumpsys wifi | grep 'MAC: '" "MAC: " "Supplicant state: COMPLETED" ||
        try_wifi_source "$serial" "ip link show wlan0" "link/ether " ||
        try_wifi_source "$serial" "cat /sys/class/net/wlan0/address" "" ||
        try_wifi_source "$serial" "ifconfig wlan0" "HWaddr "
}

# get_wifi_mac에서 보관한 출력 중 MAC이 나온 줄에서 SSID 추출 (없으면 빈 값)
extract_wifi_ssid() {
    local ssid
    [ -z "$G_WIFI_MAC" ] && return
    ssid=$(echo "$G_WIFI_STATUS" | grep -i "MAC: $G_WIFI_MAC" | head -1 |
        grep -oE '(^|[ ,])SSID: ("[^"]*"|[^,]*)' | head -1 |
        sed -E 's/^.*SSID: //; s/^"//; s/"$//')
    [[ "$ssid" == "<unknown ssid>" ]] && return
    echo "$ssid"
}

# 저장된 공장 Wi-Fi MAC 조회 (Wi-Fi 꺼짐과 무관, 미노출 기기는 빈 값)
get_factory_mac() {
    local serial="$1"
    normalize_mac "$(adb -s "$serial" shell "dumpsys wifi 2>/dev/null | grep -m1 wifi_sta_factory_mac_address" 2>/dev/null |
        sed 's/.*=//')"
}

# 첫 옥텟의 locally administered bit(0x02)로 랜덤 MAC 판정
is_randomized_mac() { (( 0x${1%%:*} & 0x02 )); }

# Bluetooth MAC 조회 (실패 시 빈 값)
get_bt_mac() {
    local serial="$1" mac
    mac=$(normalize_mac "$(adb -s "$serial" shell settings get secure bluetooth_address 2>/dev/null)") ||
        mac=$(normalize_mac "$(adb -s "$serial" shell dumpsys bluetooth_manager 2>/dev/null |
            grep -i -m1 '^[[:space:]]*address:' | awk '{print $2}')")
    echo "$mac"
}

# ─────────────────────────────────────────────────────
# 출력
# ─────────────────────────────────────────────────────

# 단일 디바이스 상세 출력
print_mac_detail() {
    local serial="$1" wifi="$2" ssid="$3" factory="$4" bt="$5"
    local note=""

    echo
    echo -e "${BOLD}$(pretty_device "$serial")${NC}"

    if [ -n "$wifi" ]; then
        is_randomized_mac "$wifi" && note="randomized"
        [ -n "$ssid" ] && note="${note:+$note · }SSID \"$ssid\""
        printf "  %-16s %s" "Wi-Fi (current)" "$wifi"
        [ -n "$note" ] && echo -ne "   ${DIM}(${note})${NC}"
        echo
    else
        printf "  %-16s %s" "Wi-Fi (current)" "n/a"
        echo -e "   ${DIM}(Wi-Fi is off or not connected)${NC}"
    fi

    printf "  %-16s %s\n" "Wi-Fi (factory)" "${factory:-n/a}"
    printf "  %-16s %s\n" "Bluetooth" "${bt:-n/a}"

    if [ -z "$factory" ] && [ -n "$wifi" ] && is_randomized_mac "$wifi"; then
        echo -e "  ${DIM}↳ Factory MAC: set Wi-Fi privacy to 'Use device MAC' and run again${NC}"
    fi
}

# ─────────────────────────────────────────────────────
# 커맨드 본체
# ─────────────────────────────────────────────────────

cmd_mac() {
    local opt_multi=0

    # 옵션 파싱
    while [ $# -gt 0 ]; do
        case "$1" in
            -h|--help)
                show_help_mac
                ;;
            -m) opt_multi=1 ;;
            -*)
                echo -e "${ERROR} Invalid option: $1"
                echo "Try 'ak mac --help' for more information."
                exit 1
                ;;
            *)
                echo -e "${ERROR} Unexpected argument: $1"
                echo "Try 'ak mac --help' for more information."
                exit 1
                ;;
        esac
        shift
    done

    # 대상 디바이스 결정
    local -a serials
    if [ $opt_multi -eq 1 ]; then
        serials=($(adb devices | grep 'device$' | cut -f1))
        if [ ${#serials[@]} -eq 0 ]; then
            echo -e "${ERROR} No connected devices found."
            exit 1
        fi
    else
        find_and_select_device
        serials=("$G_SELECTED_DEVICE")
    fi

    # 조회
    local serial wifi ssid factory bt
    for serial in "${serials[@]}"; do
        get_wifi_mac "$serial"
        wifi="$G_WIFI_MAC"
        ssid=$(extract_wifi_ssid)
        factory=$(get_factory_mac "$serial")
        bt=$(get_bt_mac "$serial")
        print_mac_detail "$serial" "$wifi" "$ssid" "$factory" "$bt"
    done
    echo
}
