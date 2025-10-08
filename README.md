# chulbong- :kr:
<p align="center">
  <img width="100" src="https://github.com/Alfex4936/chulbong-kr/assets/2356749/6236863a-11e1-45d5-b2e2-9bcf40363e1d" alt="k-pullup logo"/></br>
  <img width="1024" alt="2025-10-08 pullupbars" src="https://github.com/user-attachments/assets/0eec5b32-553b-4741-a76e-0712e5bb08f3" />
  <img alt="GitHub commit activity (branch)" src="https://img.shields.io/github/commit-activity/w/Alfex4936/chulbong-kr/main">
  <img alt="Go)" src="https://img.shields.io/badge/Go-1.2x-lightblue">
  <img alt="MySQL" src="https://img.shields.io/badge/MySQL-8.x-brightgreen">
  <img alt="Redis" src="https://img.shields.io/badge/Dragonfly-1.x-red">
</p>

> [!NOTE]  
> The service is running with [Go as backend](https://github.com/Alfex4936/chulbong-kr/tree/main/backend) but [Spring boot 3](https://github.com/Alfex4936/chulbong-kr/tree/main/backend-spring) is also available. (WIP)

<p align="center">
  <img width="356" src="https://github.com/user-attachments/assets/78072f29-59f5-4303-b241-533b9198b18d" alt="gochulbong"/></br>
</p>

### 프로젝트 소개 :world_map:

**k-pullup**은 공공장소에 있는 턱걸이 바를 찾고 공유하기 위한 커뮤니티 플랫폼입니다.

지도 API를 활용하여 사용자는 가입 및 로그인 후 턱걸이 바의 위치를 마커로 추가할 수 있으며,

사진 한 장과 간단한 설명을 함께 업로드할 수 있습니다.

로그인한 다른 사용자는 해당 마커에 댓글을 남길 수 있어, 정보 공유 및 소통이 용이합니다.

- 백엔드
  - https://github.com/Alfex4936/chulbong-kr
- 프론트엔드
  - https://github.com/2YH02/k-pullup

![0](https://github.com/Alfex4936/chulbong-kr/assets/2356749/c0f58f73-d568-4ef7-8fa1-1f20820b8fff)

## 🖼️ 구현 UI

|      메인 화면      |      검색 (주변 검색, 초성 지원)      |      위치 채팅      |
|:-------------------:|:----------------------------------:|:-------------------:|
| <img width="350" alt="main" src="https://github.com/user-attachments/assets/be4dfef3-c42b-4dc9-9df8-e10069ce35d2"> | <img width="350" alt="search" src="https://github.com/user-attachments/assets/22875f67-c5d3-43d4-8164-74cebeb54f63"> | <img width="350" alt="chat" src="https://github.com/user-attachments/assets/6a20b8b7-8ab6-4cfe-b3fd-49e22348a033"> |

|  거리뷰 (가장 가까운 위치)  |      댓글      |    오프라인 저장 (카카오맵)     |
|:-------------------:|:-------------:|:-----------------------------:|
| <img width="350" alt="road view" src="https://github.com/user-attachments/assets/7b7e7f6c-2cc1-4175-8153-a8a3727bc5a5"> | <img width="350" alt="comment" src="https://github.com/user-attachments/assets/ac60be05-07a4-4a39-be45-346e5727f079"> | <img width="350" alt="offline-pdf" src="https://github.com/Alfex4936/chulbong-kr/assets/2356749/a7eb993e-f847-40c8-9b93-d367f4c6a3f8"> |

|   공유, 북마크   |  이미지 상세  |    위치 등록    |
|:----------------:|:-------------:|:---------------------:|
| <img width="350" alt="share bookmark" src="https://github.com/user-attachments/assets/45211feb-32d1-4452-9472-f7436efa5115"> | <img width="350" alt="image detail" src="https://github.com/user-attachments/assets/ea5efb8d-c5c6-4739-8a9a-50224b1db051"> | <img width="350" alt="set location" src="https://github.com/user-attachments/assets/a05f7f3b-b2ce-467d-8e84-0b1dfd8faa72"> |

|   위치 삭제   |    지도 이동   |  마이페이지, 설정  |
|:-------------:|:-------------:|:--------------------:|
| <img width="350" alt="delete location" src="https://github.com/user-attachments/assets/23b9c5f2-9aea-4a17-81c0-ee3e6757144a"> | <img width="350" alt="move map" src="https://github.com/user-attachments/assets/f8b3784a-4c7a-49ef-8998-4c53d1be949f"> | <img width="350" alt="mypage config" src="https://github.com/user-attachments/assets/5b069293-5501-450a-841c-6fc1449e4386"> |

|   정보 수정 요청   |   수정 요청 승인  |  반응형  |
|:-------------:|:-------------:|:-------------:|
| <img width="350" alt="report" src="https://github.com/user-attachments/assets/782de329-311f-457f-b40b-d8686a7a26cd"> | <img width="350" alt="approve report" src="https://github.com/user-attachments/assets/22735cf2-c18a-40ec-8538-29a9bf4f9a6e"> | <img width="350" alt="responsive web" src="https://github.com/user-attachments/assets/9435ac37-6d47-4998-a2b0-33afce3b2c29"> |

![slack](https://github.com/Alfex4936/chulbong-kr/assets/2356749/5ec03f6a-871f-4556-90c3-13bb44769f13)

### 기능

```mermaid
sequenceDiagram
    participant User
    participant AuthService as Auth Service
    participant MarkerService as Marker Service
    participant CommentService as Comment Service
    participant ShareService as Share Service
    participant SearchService as Search Service
    participant AdminService as Admin Service
    participant ChatService as Chat Service
    participant PopularService as Popular Service
    participant OfflineService as Offline Service
    participant AddressService as Address Service

    Note over User, AuthService: User Registration and Authentication
    User->>AuthService: Sign Up with Email
    AuthService->>User: Send Email Verification
    User->>AuthService: Verify Email
    User->>AuthService: Login
    AuthService->>User: Authentication Token

    Note over User, MarkerService: Marker Management
    User->>MarkerService: Add Marker with Location, Photo, Description
    MarkerService->>AddressService: Validate Address
    AddressService-->>MarkerService: Address Validated/Failed
    MarkerService-->>User: Marker Added
    User->>MarkerService: View Marker
    MarkerService-->>User: Display Marker Details

    Note over User, CommentService: Commenting on Markers
    User->>CommentService: Add Comment to Marker
    CommentService-->>User: Comment Added
    User->>CommentService: View Comments
    CommentService-->>User: Display Comments

    Note over User, ShareService: Sharing Markers
    User->>ShareService: Share Marker Link
    ShareService-->>User: Marker Link

    Note over User, SearchService: Searching for Nearby Markers
    User->>SearchService: Search Markers Near Current Location
    SearchService-->>User: Display Nearby Markers

    Note over Admin, AdminService: Admin Tasks
    Admin->>AdminService: Review Markers with Dislikes
    AdminService-->>Admin: Display Markers for Review
    Admin->>AdminService: Update Marker Status
    AdminService-->>Admin: Marker Status Updated

    Note over User, ChatService: Chat Functionality
    User->>ChatService: Join Marker Chat Room
    ChatService-->>User: Joined Chat Room
    User->>ChatService: Send Message in Chat Room
    ChatService-->>User: Message Sent

    Note over User, PopularService: Viewing Popular Markers
    User->>PopularService: View Popular Markers
    PopularService-->>User: Display Popular Markers

    Note over User, OfflineService: Offline Marker Access
    User->>OfflineService: Download Static Map Image
    OfflineService-->>User: Static Map Image with Markers

    Note over User, MarkerService: Suggesting Marker Edits
    User->>MarkerService: Suggest Marker Edit with Photo
    MarkerService-->>User: Edit Suggestion Submitted
```

- **회원가입 및 로그인**: 사용자 인증을 위한 기본적인 회원가입 및 로그인 기능. (이메일 인증 or 소셜 로그인)
  - Google/Naver/Kakao 지원
- **마커 추가**: 턱걸이 바의 위치를 지도에 마커로 표시. 사진과 간단한 설명을 포함할 수 있음.
- **댓글 기능**: 로그인한 사용자는 각 마커에 댓글을 남길 수 있어 정보 공유가 가능.
- **마커 공유**: 특정 마커 공유 버튼을 눌러서 링크를 공유가 가능.
- **근처 턱걸이 바 검색**: 현재 화면 중앙 위치에서 가까운 턱걸이 바를 찾을 수 있는 기능.
- **필터링**: 신뢰 기반이지만 기본 필터링을 거침
  - 주소 없는 경우 DB 기록
  - 대한민국 육지 내에서만 가능
  - 사진 AI 모델
  - 위치 추가 제한 구역 설정
  - 욕설
- **채팅**: 각 마커마다 채팅 방 + 지역별 채팅 방 (익명)
- **인기 장소 확인**: 사용자들이 실시간 자주 방문하는 인기 턱걸이 바 위치 확인 기능. (현재 위치 기준 + 전국)
- **정적 이미지 오프라인**: 오프라인 용도로 철봉 위치들을 PDF로 저장할 수 있는 기능. (카카오맵 정적 이미지 보완)
- **마커 장소 검색**: 등록된 마커들의 주소 검색 기능
  - 초성 검색: ㅅㅇㅅ -> "수원시"
  - 전국 5대 지하철 역 검색 가능: "서울대입구역" -> 주변 2km 반경 철봉들 불러옴
  - QWERTY 한글: rPfyd -> "계룡"
- **정보 수정 제안**: 등록된 마커들의 정보 수정 제안 기능 (사진 1장 필수)

### TODO 아이디어

- **커뮤니티 포럼**: 사용자들이 운동 팁, 턱걸이 바 추천 등을 공유할 수 있는 커뮤니티 공간.
- **이벤트 및 챌린지**: 사용자들이 참여할 수 있는 운동 관련 이벤트 및 챌린지 개최.

### pullup/dips bar detection

"A nimble AI model, trained on 700 images in August 2024, striving to expertly detect bars with precision." (YOLO v8)

![image](https://github.com/user-attachments/assets/d822d93a-9985-480f-acfc-ba44eb4e96dc)

### 기술 스택

![image](https://github.com/Alfex4936/chulbong-kr/assets/2356749/f82e2295-ce31-4b48-af92-20a8471b7155)

https://github.com/Alfex4936/chulbong-kr/assets/2356749/913b113c-4a8d-4df1-bb5a-83f6babf7475

- **백엔드**: Go언어 Fiber v2, MySQL, AWS S3, LavinMQ (RabbitMQ), Redis, Bleve (Apache Lucene-like, ZincSearch에서 직접 검색 인덱싱으로 변경)
  - 메인: Go, 서브: Java (전체 프로젝트 자바로도 작성 중)
- **프론트엔드**: NextJS (TypeScript), Tailwind css, Storybook, Zustand, Yarn Berry
- **개발 & 운영 효율성**: pprof, flamegraph, Uber's zap logger, Swagger OpenAPI, Prometheus+Grafana
- **AI**: YOLO v8, gpt-4o mini
- **협업**: Slack (+ Slack API)

### 🚀 Project Roles 🚀

- **Backend Development**  
  👨‍💻 [@Alfex4936](https://github.com/Alfex4936)

- **Frontend Development**  
  🎨 [@2YH02](https://github.com/2YH02)

> [!NOTE]
> 대부분 철봉 위치 데이터는 [chulbong.kr](https://chulbong.kr/) 에서 가져왔음을 알립니다. (2차 필터링)

### Data analysis (2024 June ~ 2024 Nov)
![graph_animated](https://github.com/user-attachments/assets/7c990f2c-d618-4403-9330-35fa2f553c14)
