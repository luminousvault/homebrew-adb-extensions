#!/bin/bash
#@@BUILD_EXCLUDE_START
# ═══════════════════════════════════════════════════
# Device Management
# ADB 디바이스 관리 함수
# ═══════════════════════════════════════════════════
#@@BUILD_EXCLUDE_END

# 디바이스 정보 출력 함수
# 표시 레벨: minimal(브랜드 모델명) | short(+ID) | normal(+Android/API)
pretty_device() {
    local device_id="$1"
    local level="${2:-normal}"  # 기본값 normal
    local props brand model version api
    local device_status error_reason

    # 디바이스 상태 확인
    device_status=$(adb devices | grep "$device_id" | awk '{print $2}')
    if [[ "$device_status" == "unauthorized" ]] || [[ "$device_status" == "offline" ]]; then
        echo "($device_id)"
        return
    fi

    # getprop 명령 실행
    props=$(adb -s "$device_id" shell getprop 2>/dev/null)
    
    # getprop 실패 시 에러 처리
    if [ -z "$props" ]; then
        error_reason="device not responding"
        # minimal 레벨이 아닌 경우에만 디바이스 ID와 에러 이유 표시
        if [[ "$level" == "minimal" ]]; then
            echo "Unknown Device"
        else
            echo "Unknown Device ($device_id): $error_reason"
        fi
        return
    fi

    brand=$(echo "$props" | awk -F'[][]' '$2 == "ro.product.brand" {print $4}' | tr -d '\r\n')
    # 변경 후: bash 3.2 호환 (awk 사용)
    brand=$(echo "$brand" | awk '{print toupper(substr($0,1,1)) tolower(substr($0,2))}')
    model=$(echo "$props" | awk -F'[][]' '$2 == "ro.product.model" {print $4}' | tr -d '\r\n')
    version=$(echo "$props" | awk -F'[][]' '$2 == "ro.build.version.release" {print $4}' | tr -d '\r\n')
    api=$(echo "$props" | awk -F'[][]' '$2 == "ro.build.version.sdk" {print $4}' | tr -d '\r\n')
    
    # 빈 값 처리
    [ -z "$brand" ] && brand="Unknown"
    [ -z "$model" ] && model="Device"
    [ -z "$version" ] && version="Unknown"
    [ -z "$api" ] && api="Unknown"

    case "$level" in
        minimal)
            echo "$brand $model"
            ;;
        short)
            echo "$brand $model ($device_id)"
            ;;
        *)  # normal (default)
            echo "$brand $model ($device_id) Android $version, API $api"
            ;;
    esac
}

# ─────────────────────────────────────────────────────
# 단일 디바이스 선택 (대부분의 커맨드용)
# ─────────────────────────────────────────────────────

# 연결된 디바이스 찾기 및 선택 (단일 디바이스)
# 결과: G_SELECTED_DEVICE 변수에 선택된 디바이스 ID 저장
find_and_select_device() {
    local devices
    devices=$(adb devices | grep 'device$' | cut -f1)
    local device_array=($devices)
    local device_count=${#device_array[@]}

    case $device_count in
        0 ) # 연결된 장치가 없을 경우 에러 메시지 출력
            echo -e "${ERROR} No connected devices found."; exit 1 ;;
        1 ) # 연결된 장치가 하나일 경우 해당 장치 선택
            G_SELECTED_DEVICE="${device_array[0]}" ;;
        * ) # 여러 장치가 연결된 경우 사용자에게 선택지 제공
            present_device_selection_single "$devices"
            ;;
    esac
}

# 단일 디바이스 선택 UI
present_device_selection_single() {
    local devices="$1"
    declare -a device_list
    
    # 디바이스 목록을 줄 단위로 분리하여 배열에 저장
    IFS=$'\n' read -rd '' -a device_list <<< "$devices"
    
    # 디바이스 정보를 pretty_device로 포맷팅한 배열 생성
    local -a formatted_devices=()
    for device_info in "${device_list[@]}"; do
      formatted_devices+=("$(pretty_device $device_info)")
    done
    
    # 인터랙티브 단일 선택 실행 (필터 비활성화)
    select_interactive "single:nofilter" "Select a device" "${formatted_devices[@]}"
    
    # 선택된 인덱스를 사용하여 실제 디바이스 ID 설정
    G_SELECTED_DEVICE="${device_list[$SELECTED_INDEX]}"
}

# ─────────────────────────────────────────────────────
# 멀티 디바이스 선택 (install 커맨드용)
# ─────────────────────────────────────────────────────

# 연결된 디바이스 찾기 및 선택 (멀티 디바이스 지원)
# 결과: selected_device 배열에 선택된 디바이스 ID들 저장
find_and_select_devices_multi() {
    local opt_m_used=$1  # -m 옵션 사용 여부
    local devices
    devices=$(adb devices | grep 'device$' | cut -f1)
    local device_array=($devices)
    local device_count=${#device_array[@]}

    case $device_count in
        0 ) # 연결된 장치가 없을 경우 에러 메시지 출력
            echo -e "${ERROR} No connected devices found."; exit 1 ;;
        1 ) # 연결된 장치가 하나일 경우 해당 장치 선택
            selected_device=("${device_array[0]}") ;;
        * ) # 여러 장치가 연결된 경우
            # -m 옵션이 있는 경우 모든 디바이스를 선택
            if [ $opt_m_used -eq 1 ]; then
                selected_device=("${device_array[@]}")
            else
                present_device_selection_multi "$devices"
            fi
            ;;
    esac
}

# 멀티 디바이스 선택 UI
present_device_selection_multi() {
    local devices="$1"
    declare -a device_list
    
    # 디바이스 목록을 줄 단위로 분리하여 배열에 저장
    IFS=$'\n' read -rd '' -a device_list <<< "$devices"
    
    # 디바이스 정보를 pretty_device로 포맷팅한 배열 생성
    local -a formatted_devices=()
    for device_info in "${device_list[@]}"; do
      formatted_devices+=("$(pretty_device $device_info)")
    done
    
    # 인터랙티브 멀티 선택 실행 (필터 비활성화)
    select_interactive "multi:nofilter" "Select devices for installation" "${formatted_devices[@]}"
    
    # 선택된 인덱스를 사용하여 실제 디바이스 ID 배열 생성
    selected_device=()
    for idx in "${SELECTED_INDICES[@]}"; do
      selected_device+=("${device_list[$idx]}")
    done
}

# ─────────────────────────────────────────────────────
# 디바이스 목록 출력 (devices 커맨드용)
# ─────────────────────────────────────────────────────

# 연결된 디바이스를 테이블 형식으로 출력
find_and_list_devices() {
    local line device_id status
    local -a device_ids device_statuses
    local -a brands models versions cpus status_strings
    
    # 디바이스 목록 수집
    local devices_list=()
    while IFS= read -r line; do
        [[ "$line" =~ ^List ]] && continue
        [[ -z "$line" ]] && continue
        devices_list+=("$line")
    done < <(adb devices)
    
    if [ -z "${devices_list[*]}" ]; then
        echo -e "${RED}ERROR: No connected devices found.${NC}"
        return
    fi

    # 모든 디바이스 정보 수집
    local max_id_len=15 max_model_len=20 max_android_len=12 max_cpu_len=10 max_status_len=11
    
    for line in "${devices_list[@]}"; do
        device_id=$(echo "$line" | awk '{print $1}')
        status=$(echo "$line" | awk '{print $2}')
        
        device_ids+=("$device_id")
        device_statuses+=("$status")
        
        # 디바이스 ID 길이 업데이트
        local id_len=${#device_id}
        [ $id_len -gt $max_id_len ] && max_id_len=$id_len
        
        case "$status" in
            device)
                local props brand model version api cpu status_str
                props=$(adb -s "$device_id" shell getprop 2>/dev/null)
                
                # getprop 실패 시 기본값 설정
                if [ -z "$props" ]; then
                    brand="Unknown"
                    model="Device"
                    version="Unknown"
                    api="Unknown"
                    cpu="Unknown"
                else
                    brand=$(echo "$props" | awk -F'[][]' '$2 == "ro.product.brand" {print $4}' | tr -d '\r\n')
                    brand=$(echo "$brand" | awk '{print toupper(substr($0,1,1)) tolower(substr($0,2))}')
                    model=$(echo "$props" | awk -F'[][]' '$2 == "ro.product.model" {print $4}' | tr -d '\r\n')
                    cpu=$(echo "$props" | awk -F'[][]' '$2 == "ro.product.cpu.abi" {print $4}' | tr -d '\r\n')
                    version=$(echo "$props" | awk -F'[][]' '$2 == "ro.build.version.release" {print $4}' | tr -d '\r\n')
                    api=$(echo "$props" | awk -F'[][]' '$2 == "ro.build.version.sdk" {print $4}' | tr -d '\r\n')
                    
                    # 빈 값 처리
                    [ -z "$brand" ] && brand="Unknown"
                    [ -z "$model" ] && model="Device"
                    [ -z "$version" ] && version="Unknown"
                    [ -z "$api" ] && api="Unknown"
                    [ -z "$cpu" ] && cpu="Unknown"

                    # 32/64비트 지원 여부 표시 (abilist는 Android 5.0+에만 존재)
                    local abilist32 abilist64
                    abilist32=$(echo "$props" | awk -F'[][]' '$2 == "ro.product.cpu.abilist32" {print $4}' | tr -d '\r\n')
                    abilist64=$(echo "$props" | awk -F'[][]' '$2 == "ro.product.cpu.abilist64" {print $4}' | tr -d '\r\n')
                    if [[ -n "$abilist64" && -n "$abilist32" ]]; then
                        cpu="$cpu (64/32)"
                    elif [[ -n "$abilist64" ]]; then
                        cpu="$cpu (64only)"
                    elif [[ -n "$abilist32" ]]; then
                        cpu="$cpu (32only)"
                    fi
                fi
                
                brands+=("$brand")
                models+=("$brand $model")
                versions+=("$version (API $api)")
                cpus+=("$cpu")
                status_strings+=("Connected")
                
                # 길이 업데이트 (배열의 마지막 요소 접근)
                local model_len=${#models[${#models[@]}-1]}
                [ $model_len -gt $max_model_len ] && max_model_len=$model_len
                local android_len=${#versions[${#versions[@]}-1]}
                [ $android_len -gt $max_android_len ] && max_android_len=$android_len
                local cpu_len=${#cpus[${#cpus[@]}-1]}
                [ $cpu_len -gt $max_cpu_len ] && max_cpu_len=$cpu_len
                ;;
            unauthorized)
                brands+=("")
                models+=("-")
                versions+=("-")
                cpus+=("-")
                status_strings+=("Unauthorized")
                ;;
            offline)
                brands+=("")
                models+=("-")
                versions+=("-")
                cpus+=("-")
                status_strings+=("Offline")
                ;;
            *)
                brands+=("")
                models+=("-")
                versions+=("-")
                cpus+=("-")
                status_strings+=("Unknown")
                ;;
        esac
    done

    echo
    echo -e "${CYAN}${BOLD}Connected Devices:${NC}"
    echo
    
    # 테이블 헤더 (차분한 색상)
    local header_id_pad=$((max_id_len - 9))  # "Device ID" 길이 = 9
    local header_model_pad=$((max_model_len - 5))  # "Model" 길이 = 5
    local header_android_pad=$((max_android_len - 7))  # "Android" 길이 = 7
    local header_cpu_pad=$((max_cpu_len - 3))  # "CPU" 길이 = 3
    local header_status_pad=$((max_status_len - 6))  # "Status" 길이 = 6
    
    echo -ne "${DIM}${BOLD}Device ID${NC}"
    printf "%${header_id_pad}s  " ""
    echo -ne "${DIM}${BOLD}Model${NC}"
    printf "%${header_model_pad}s  " ""
    echo -ne "${DIM}${BOLD}Android${NC}"
    printf "%${header_android_pad}s  " ""
    echo -ne "${DIM}${BOLD}CPU${NC}"
    printf "%${header_cpu_pad}s  " ""
    echo -e "${DIM}${BOLD}Status${NC}"
    
    # 구분선
    separator=""
    total_width=$((max_id_len + max_model_len + max_android_len + max_cpu_len + max_status_len + 8))
    i=0
    while [ $i -lt $total_width ]; do
        separator="${separator}─"
        i=$((i + 1))
    done
    echo -e "${DIM}${separator}${NC}"
    
    # 테이블 데이터
    i=0
    while [ $i -lt ${#device_ids[@]} ]; do
        device_id="${device_ids[$i]}"
        model="${models[$i]}"
        version="${versions[$i]}"
        cpu="${cpus[$i]}"
        status_str="${status_strings[$i]}"
        
        # 색상 적용 (차분하게)
        colored_id="${BOLD}${device_id}${NC}"
        colored_model="${model}"
        colored_version="${version}"
        colored_cpu="${cpu}"
        # 64비트 전용 디바이스는 32비트 앱 설치 불가 → 강조
        [[ "$cpu" == *"(64only)" ]] && colored_cpu="${YELLOW}${cpu}${NC}"

        case "$status_str" in
            Connected)
                colored_status="${status_str}"
                ;;
            Unauthorized)
                colored_status="${RED}${status_str}${NC}"
                ;;
            Offline)
                colored_status="${DIM}${status_str}${NC}"
                ;;
            *)
                colored_status="${DIM}${status_str}${NC}"
                ;;
        esac
        
        # printf로 정렬 (색상 코드 길이 보정)
        id_padding=$((max_id_len - ${#device_id}))
        model_padding=$((max_model_len - ${#model}))
        version_padding=$((max_android_len - ${#version}))
        cpu_padding=$((max_cpu_len - ${#cpu}))
        status_padding=$((max_status_len - ${#status_str}))
        
        local formatted_line
        formatted_line=$(printf "%s%${id_padding}s  %s%${model_padding}s  %s%${version_padding}s  %s%${cpu_padding}s  %s%${status_padding}s" \
            "$colored_id" "" \
            "$colored_model" "" \
            "$colored_version" "" \
            "$colored_cpu" "" \
            "$colored_status")
        echo -e "$formatted_line"
        
        i=$((i + 1))
    done
    echo
}

# 선택된 디바이스들을 출력하는 함수 (install 커맨드용)
pretty_print_selected_devices() {
  echo -e "${BARROW} ${BOLD}Selected devices for installation:${NC}"
  local i=1
  for device in "${selected_device[@]}"; do
    echo "${i}. $(pretty_device $device)"
    ((i++))
  done
}
