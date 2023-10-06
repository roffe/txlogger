package kwp2000

const (
	/* DIAGNOSTIC MANAGEMENT FUNCTIONAL UNIT */
	START_DIAGNOSTIC_SESSION = 0x10 // present in Trionic 7
	STOP_DIAGNOSTIC_SESSION  = 0x20 // present in Trionic 7
	SECURITY_ACCESS          = 0x27 // present in Trionic 7
	TESTER_PRESENT           = 0x3E // present in Trionic 7
	ECU_RESET                = 0x11 // present in Trionic 7
	READ_ECU_IDENTIFICATION  = 0x1A // present in Trionic 7

	/* DATA TRANSMISSION FUNCTIONAL UNIT */
	READ_DATA_BY_LOCAL_IDENTIFIER       = 0x21 // present in Trionic 7
	READ_DATA_BY_COMMON_IDENTIFIER      = 0x22
	READ_MEMORY_BY_ADDRESS              = 0x23 // present in Trionic 7
	DYNAMICALLY_DEFINE_LOCAL_IDENTIFIER = 0x2C // present in Trionic 7
	WRITE_DATA_BY_LOCAL_IDENTIFIER      = 0x3B // present in Trionic 7
	WRITE_DATA_BY_COMMON_IDENTIFIER     = 0x2E
	WRITE_MEMORY_BY_ADDRESS             = 0x3D // present in Trionic 7
	SET_DATA_RATES                      = 0x26

	/* STORED DATA TRANSMISSION FUNCTIONAL UNIT */
	READ_DIAGNOSTIC_TROUBLE_CODES           = 0x13
	READ_DIAGNOSTIC_TROUBLE_CODES_BY_STATUS = 0x18 // present in Trionic 7
	READ_STATUS_OF_DIAGNOSTIC_TROUBLE_CODES = 0x17
	READ_FREEZEFRAME_DATA                   = 0x12 // present in Trionic 7
	CLEAR_DIAGNOSTIC_INFORMATION            = 0x14 // present in Trionic 7

	/* INPUTOUTPUT CONTROL FUNCTIONAL UNIT */
	INPUT_OUTPUT_CONTROL_BY_COMMON_IDENTIFIER = 0x2F
	INPUT_OUTPUT_CONTROL_BY_LOCAL_IDENTIFIER  = 0x30 // present in Trionic 7

	/* REMOTE ACTIVATION OF ROUTINE FUNCTIONAL UNIT */
	START_ROUTINE_BY_LOCAL_IDENTIFIER           = 0x31 // present in Trionic 7
	START_ROUTINE_BY_ADDRESS                    = 0x38
	STOP_ROUTINE_BY_LOCAL_IDENTIFIER            = 0x32
	STOP_ROUTINE_BY_ADDRESS                     = 0x39
	REQUEST_ROUTINE_RESULTS_BY_LOCAL_IDENTIFIER = 0x33 // present in Trionic 7
	REQUEST_ROUTINE_RESULTS_BY_ADDRESS          = 0x3A

	/* UPLOAD DOWNLOAD FUNCTIONAL UNIT */
	REQUEST_DOWNLOAD      = 0x34 // present in Trionic 7
	REQUEST_UPLOAD        = 0x35 // present in Trionic 7
	TRANSFER_DATA         = 0x36 // present in Trionic 7
	REQUEST_TRANSFER_EXIT = 0x37 // present in Trionic 7

	STOP_REPEATED_DATA_TRANSMISSION = 0x25
	START_COMMUNICATION             = 0x81
	STOP_COMMUNICATION              = 0x82
	ACCESS_TIMING_PARAMETERS        = 0x83

	SYMBOL_IDENTIFICATION = 0x80

	/* Dynamically defined local identifier constants */
	DM_DBLI  = 0x01
	DM_DBMA  = 0x03
	DM_CDDLI = 0x04
)
