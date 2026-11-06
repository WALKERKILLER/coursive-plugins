package provider

import "time"

// Capability groups a provider may announce.
const (
	CapSlides     = "slides"
	CapCourseware = "courseware"
	CapLMS        = "lms"
)

// Field is one input of the sign-in form.
type Field struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	// Kind is "text", "password" or "otp".
	Kind string `json:"kind"`
	// Remember false means the value is used once and never stored.
	Remember *bool `json:"remember,omitempty"`
}

// Init is the answer to initialize.
type Init struct {
	Protocol     string   `json:"protocol"`
	Capabilities []string `json:"capabilities"`
	SignIn       struct {
		Fields []Field `json:"fields"`
	} `json:"sign_in"`
}

// Sign-in states.
const (
	SignedOut  = "signed_out"
	SignedIn   = "signed_in"
	NeedsInput = "needs_input"
	Expired    = "expired"
)

// Challenge is an extra sign-in step, such as a captcha image.
type Challenge struct {
	ID string `json:"id"`
	// Kind is "image", "text" or "qr".
	Kind   string `json:"kind"`
	Prompt string `json:"prompt"`
	// Image is a data: URL.
	Image string `json:"image,omitempty"`
}

// Auth is the sign-in state.
type Auth struct {
	State     string     `json:"state"`
	Account   *Account   `json:"account,omitempty"`
	Challenge *Challenge `json:"challenge,omitempty"`
}

// Account names who is signed in, for display only.
type Account struct {
	Display string `json:"display"`
}

// Teacher is one teacher of a course.
type Teacher struct {
	Username string `json:"username"`
	Realname string `json:"realname"`
}

// Course is a platform course.
type Course struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Teacher       string    `json:"teacher,omitempty"`
	Teachers      []Teacher `json:"teachers,omitempty"`
	College       string    `json:"college,omitempty"`
	Term          string    `json:"term,omitempty"`
	Thumb         string    `json:"thumb,omitempty"`
	StartAt       int64     `json:"start_at,omitempty"`
	EndAt         int64     `json:"end_at,omitempty"`
	LectureTotal  int       `json:"lecture_total,omitempty"`
	Progress      float64   `json:"progress,omitempty"`
	LastLectureID string    `json:"last_lecture_id,omitempty"`
	LastLearnAt   string    `json:"last_learn_at,omitempty"`
	URL           string    `json:"url,omitempty"`
}

// Lecture is one recorded class.
type Lecture struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Date        string `json:"date,omitempty"`
	BeginAt     int64  `json:"begin_at,omitempty"`
	EndAt       int64  `json:"end_at,omitempty"`
	Lecturer    string `json:"lecturer,omitempty"`
	Room        string `json:"room,omitempty"`
	HasPlayback bool   `json:"has_playback"`
	URL         string `json:"url,omitempty"`
}

// Address is where the daemon fetches something; headers stay in the daemon.
type Address struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt int64             `json:"expires_at,omitempty"`
}

// Stream is one playable view of a lecture.
type Stream struct {
	ID string `json:"id"`
	// Kind is "camera", "screen" or "audio".
	Kind  string `json:"kind,omitempty"`
	Label string `json:"label"`
	Address
	Thumb    string `json:"thumb,omitempty"`
	Duration int64  `json:"duration,omitempty"`
}

// Media is a lecture's streams.
type Media struct {
	Streams  []Stream `json:"streams"`
	Duration int64    `json:"duration,omitempty"`
	Seekable bool     `json:"seekable"`
}

// Slide is one slide snapshot.
type Slide struct {
	TSec     int64             `json:"t_sec"`
	URL      string            `json:"url"`
	ThumbURL string            `json:"thumb_url,omitempty"`
	Key      bool              `json:"key,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// File is a courseware or learning-platform file.
type File struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Folder    string `json:"folder,omitempty"`
	Size      int64  `json:"size,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Address
}

// LMSCourse is a course on the learning platform.
type LMSCourse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Code  string `json:"code,omitempty"`
	Term  string `json:"term,omitempty"`
}

// Assignment is a learning-platform assignment.
type Assignment struct {
	ID        string     `json:"id"`
	Course    string     `json:"course"`
	Title     string     `json:"title"`
	DueAt     *time.Time `json:"due_at,omitempty"`
	URL       string     `json:"url,omitempty"`
	Points    *float64   `json:"points,omitempty"`
	Submitted bool       `json:"submitted"`
}

// Announcement is a learning-platform announcement.
type Announcement struct {
	ID       string     `json:"id"`
	Course   string     `json:"course"`
	Title    string     `json:"title"`
	HTML     string     `json:"html"`
	PostedAt *time.Time `json:"posted_at,omitempty"`
	URL      string     `json:"url,omitempty"`
	Author   string     `json:"author,omitempty"`
}
