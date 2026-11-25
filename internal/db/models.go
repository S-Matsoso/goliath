// Package db defines the register's GORM models and database lifecycle helpers.
package db

import "time"

type Sex string

const (
	SexMale   Sex = "M"
	SexFemale Sex = "F"
	SexOther  Sex = "X"
)

type CitizenshipStatus string

const (
	CitizenshipCitizen           CitizenshipStatus = "CITIZEN"
	CitizenshipPermanentResident CitizenshipStatus = "PERMANENT_RESIDENT"
	CitizenshipForeign           CitizenshipStatus = "FOREIGN"
	CitizenshipStateless         CitizenshipStatus = "STATELESS"
)

type MaritalStatus string

const (
	MaritalSingle   MaritalStatus = "SINGLE"
	MaritalMarried  MaritalStatus = "MARRIED"
	MaritalDivorced MaritalStatus = "DIVORCED"
	MaritalWidowed  MaritalStatus = "WIDOWED"
)

type BiometricKind string

const (
	BiometricFinger BiometricKind = "FINGER"
	BiometricFace   BiometricKind = "FACE"
	BiometricIris   BiometricKind = "IRIS"
)

type AuditOutcome string

const (
	AuditOutcomeOK     AuditOutcome = "OK"
	AuditOutcomeDenied AuditOutcome = "DENIED"
	AuditOutcomeError  AuditOutcome = "ERROR"
)

type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "PENDING"
	DeliverySent    DeliveryStatus = "SENT"
	DeliveryFailed  DeliveryStatus = "FAILED"
)

type BackupStatus string

const (
	BackupRunning BackupStatus = "RUNNING"
	BackupOK      BackupStatus = "OK"
	BackupFailed  BackupStatus = "FAILED"
)

type Person struct {
	ID                  uint64            `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	NationalIDIndex     []byte            `gorm:"column:nid_idx;type:binary(32);not null;uniqueIndex:uq_persons_nid_idx" json:"-"`
	NationalIDEncrypted []byte            `gorm:"column:nid_enc;type:varbinary(96);not null" json:"-"`
	FirstNames          string            `gorm:"column:first_names;type:varchar(120);not null"`
	LastName            string            `gorm:"column:last_name;type:varchar(80);not null"`
	DateOfBirth         time.Time         `gorm:"column:date_of_birth;type:date;not null;index:ix_persons_dob"`
	Sex                 Sex               `gorm:"column:sex;type:enum('M','F','X');not null"`
	CitizenshipStatus   CitizenshipStatus `gorm:"column:citizenship_status;type:enum('CITIZEN','PERMANENT_RESIDENT','FOREIGN','STATELESS');not null"`
	MaritalStatus       MaritalStatus     `gorm:"column:marital_status;type:enum('SINGLE','MARRIED','DIVORCED','WIDOWED');not null;default:SINGLE"`
	DeceasedAt          *time.Time        `gorm:"column:deceased_at;type:date"`
	CreatedAt           time.Time         `gorm:"column:created_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt           time.Time         `gorm:"column:updated_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3);autoUpdateTime"`
}

func (Person) TableName() string { return "persons" }

type Address struct {
	ID         uint64     `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	PersonID   uint64     `gorm:"column:person_id;type:bigint unsigned;not null;index:ix_addr_person_current,priority:1"`
	Person     *Person    `gorm:"foreignKey:PersonID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	Line1      string     `gorm:"column:line1;type:varchar(160);not null"`
	City       string     `gorm:"column:city;type:varchar(80);not null"`
	District   string     `gorm:"column:district;type:varchar(40);not null"`
	PostalCode *string    `gorm:"column:postal_code;type:varchar(10)"`
	IsCurrent  bool       `gorm:"column:is_current;type:tinyint(1);not null;default:true;index:ix_addr_person_current,priority:2"`
	ValidFrom  time.Time  `gorm:"column:valid_from;type:date;not null"`
	ValidTo    *time.Time `gorm:"column:valid_to;type:date"`
}

func (Address) TableName() string { return "addresses" }

type Birth struct {
	ID             uint64    `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	PersonID       uint64    `gorm:"column:person_id;type:bigint unsigned;not null;uniqueIndex"`
	Person         *Person   `gorm:"foreignKey:PersonID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	PlaceOfBirth   string    `gorm:"column:place_of_birth;type:varchar(120);not null"`
	FacilityCode   *string   `gorm:"column:facility_code;type:varchar(20)"`
	MotherPersonID *uint64   `gorm:"column:mother_person_id;type:bigint unsigned"`
	FatherPersonID *uint64   `gorm:"column:father_person_id;type:bigint unsigned"`
	CertificateNo  string    `gorm:"column:certificate_no;type:varchar(30);not null;uniqueIndex"`
	RegisteredBy   uint64    `gorm:"column:registered_by;type:bigint unsigned;not null"`
	RegisteredAt   time.Time `gorm:"column:registered_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3)"`
}

func (Birth) TableName() string { return "births" }

type Death struct {
	ID            uint64    `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	PersonID      uint64    `gorm:"column:person_id;type:bigint unsigned;not null;uniqueIndex"`
	Person        *Person   `gorm:"foreignKey:PersonID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	DateOfDeath   time.Time `gorm:"column:date_of_death;type:date;not null"`
	PlaceOfDeath  *string   `gorm:"column:place_of_death;type:varchar(120)"`
	CauseCode     *string   `gorm:"column:cause_code;type:varchar(10)"`
	CertificateNo string    `gorm:"column:certificate_no;type:varchar(30);not null;uniqueIndex"`
	RegisteredBy  uint64    `gorm:"column:registered_by;type:bigint unsigned;not null"`
	RegisteredAt  time.Time `gorm:"column:registered_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3)"`
}

func (Death) TableName() string { return "deaths" }

type Marriage struct {
	ID            uint64     `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	SpouseAID     uint64     `gorm:"column:spouse_a_id;type:bigint unsigned;not null"`
	SpouseA       *Person    `gorm:"foreignKey:SpouseAID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	SpouseBID     uint64     `gorm:"column:spouse_b_id;type:bigint unsigned;not null"`
	SpouseB       *Person    `gorm:"foreignKey:SpouseBID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	MarriedOn     time.Time  `gorm:"column:married_on;type:date;not null"`
	DissolvedOn   *time.Time `gorm:"column:dissolved_on;type:date"`
	CertificateNo string     `gorm:"column:certificate_no;type:varchar(30);not null;uniqueIndex"`
}

func (Marriage) TableName() string { return "marriages" }

type BiometricTemplate struct {
	ID         uint64        `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	PersonID   uint64        `gorm:"column:person_id;type:bigint unsigned;not null;index:ix_bio_person"`
	Person     *Person       `gorm:"foreignKey:PersonID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	Kind       BiometricKind `gorm:"column:kind;type:enum('FINGER','FACE','IRIS');not null"`
	Template   []byte        `gorm:"column:template;type:mediumblob;not null" json:"-"`
	CapturedAt time.Time     `gorm:"column:captured_at;type:datetime(3);not null"`
}

func (BiometricTemplate) TableName() string { return "biometric_templates" }

type Department struct {
	Code   string `gorm:"column:code;type:varchar(20);primaryKey"`
	Name   string `gorm:"column:name;type:varchar(100);not null"`
	Active bool   `gorm:"column:active;type:tinyint(1);not null;default:true"`
}

func (Department) TableName() string { return "departments" }

type Official struct {
	ID            uint64      `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	IDPSubject    string      `gorm:"column:idp_subject;type:varchar(128);not null;uniqueIndex"`
	Department    string      `gorm:"column:department;type:varchar(20);not null"`
	DepartmentRef *Department `gorm:"foreignKey:Department;references:Code;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	Role          string      `gorm:"column:role;type:varchar(30);not null"`
	Active        bool        `gorm:"column:active;type:tinyint(1);not null;default:true"`
	CreatedAt     time.Time   `gorm:"column:created_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3)"`
}

func (Official) TableName() string { return "officials" }

type AuditLog struct {
	ID           uint64       `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	ChainID      string       `gorm:"column:chain_id;type:varchar(20);not null;uniqueIndex:uq_chain_seq,priority:1"`
	Seq          uint64       `gorm:"column:seq;type:bigint unsigned;not null;uniqueIndex:uq_chain_seq,priority:2"`
	OccurredAt   time.Time    `gorm:"column:occurred_at;type:datetime(3);not null;index:ix_audit_subject,priority:2;index:ix_audit_official,priority:2"`
	OfficialID   *uint64      `gorm:"column:official_id;type:bigint unsigned;index:ix_audit_official,priority:1"`
	Department   string       `gorm:"column:department;type:varchar(20);not null"`
	Action       string       `gorm:"column:action;type:varchar(60);not null"`
	SubjectIndex []byte       `gorm:"column:subject_idx;type:binary(32);index:ix_audit_subject,priority:1" json:"-"`
	Purpose      *string      `gorm:"column:purpose;type:varchar(120)"`
	Outcome      AuditOutcome `gorm:"column:outcome;type:enum('OK','DENIED','ERROR');not null"`
	RequestID    string       `gorm:"column:request_id;type:char(36);not null"`
	PreviousHash []byte       `gorm:"column:prev_hash;type:binary(32);not null" json:"-"`
	RowHash      []byte       `gorm:"column:row_hash;type:binary(32);not null" json:"-"`
}

func (AuditLog) TableName() string { return "audit_log" }

type EventOutbox struct {
	ID        uint64    `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	EventType string    `gorm:"column:event_type;type:varchar(40);not null"`
	PersonID  uint64    `gorm:"column:person_id;type:bigint unsigned;not null"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3);index:ix_outbox_created"`
}

func (EventOutbox) TableName() string { return "event_outbox" }

type EventSubscription struct {
	ID         uint64 `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	Department string `gorm:"column:department;type:varchar(20);not null;uniqueIndex:uq_sub,priority:1"`
	EventType  string `gorm:"column:event_type;type:varchar(40);not null;uniqueIndex:uq_sub,priority:2"`
	WebhookURL string `gorm:"column:webhook_url;type:varchar(300);not null"`
	SecretRef  string `gorm:"column:secret_ref;type:varchar(100);not null"`
	Active     bool   `gorm:"column:active;type:tinyint(1);not null;default:true"`
}

func (EventSubscription) TableName() string { return "event_subscriptions" }

type EventDelivery struct {
	ID            uint64         `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	OutboxID      uint64         `gorm:"column:outbox_id;type:bigint unsigned;not null;uniqueIndex:uq_delivery,priority:1"`
	Department    string         `gorm:"column:department;type:varchar(20);not null;uniqueIndex:uq_delivery,priority:2"`
	Status        DeliveryStatus `gorm:"column:status;type:enum('PENDING','SENT','FAILED');not null;default:PENDING;index:ix_delivery_pending,priority:1"`
	Attempts      int            `gorm:"column:attempts;type:int;not null;default:0"`
	NextAttemptAt time.Time      `gorm:"column:next_attempt_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3);index:ix_delivery_pending,priority:2"`
	LastError     *string        `gorm:"column:last_error;type:varchar(255)"`
}

func (EventDelivery) TableName() string { return "event_deliveries" }

type BackupLease struct {
	Name      string     `gorm:"column:name;type:varchar(40);primaryKey"`
	Holder    *string    `gorm:"column:holder;type:varchar(40)"`
	ExpiresAt *time.Time `gorm:"column:expires_at;type:datetime(3)"`
}

func (BackupLease) TableName() string { return "backup_leases" }

type BackupRun struct {
	ID          uint64       `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	Node        string       `gorm:"column:node;type:varchar(40);not null"`
	StartedAt   time.Time    `gorm:"column:started_at;type:datetime(3);not null"`
	FinishedAt  *time.Time   `gorm:"column:finished_at;type:datetime(3)"`
	Status      BackupStatus `gorm:"column:status;type:enum('RUNNING','OK','FAILED');not null"`
	ArtifactURI *string      `gorm:"column:artifact_uri;type:varchar(300)"`
	SHA256      *string      `gorm:"column:sha256;type:char(64)"`
	SizeBytes   *uint64      `gorm:"column:size_bytes;type:bigint unsigned"`
	BinlogFile  *string      `gorm:"column:binlog_file;type:varchar(60)"`
	BinlogPos   *uint64      `gorm:"column:binlog_pos;type:bigint unsigned"`
	Error       *string      `gorm:"column:error;type:varchar(500)"`
}

func (BackupRun) TableName() string { return "backup_runs" }
