package kwp2000

import (
	"fmt"
)

const (
	GENERAL_REJECT                                     = 0x10
	SERVICE_NOT_SUPPORTED                              = 0x11
	SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT        = 0x12
	BUSY_REPEAT_REQUEST                                = 0x21
	CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR   = 0x22
	ROUTINE_NOT_COMPLETE_OR_SERVICE_IN_PROGRESS        = 0x23
	REQUEST_OUT_OF_RANGE                               = 0x31
	SECURITY_ACCESS_DENIED_OR_REQUESTED                = 0x33
	INVALID_KEY                                        = 0x35
	EXCEED_NUMBER_OF_ATTEMPTS                          = 0x36
	REQUIRED_TIME_DELAY_NOT_EXPIRED                    = 0x37
	DOWNLOAD_NOT_ACCEPTED                              = 0x40
	IMPROPER_DOWNLOAD_TYPE                             = 0x41
	CANNOT_DOWNLOAD_TO_SPECIFIED_ADDRESS               = 0x42
	CANNOT_DOWNLOAD_NUMBER_OF_BYTES_REQUESTED          = 0x43
	UPLOAD_NOT_ACCEPTED                                = 0x50
	IMPROPER_UPLOAD_TYPE                               = 0x51
	CANNOT_UPLOAD_FROM_SPECIFIED_ADDRESS               = 0x52
	CANNOT_UPLOAD_NUMBER_OF_BYTES_REQUESTED            = 0x53
	TRANSFER_SUSPENDED                                 = 0x71
	TRANSFER_ABORTED                                   = 0x72
	ILLEGAL_ADDRESS_IN_BLOCK_TRANSFER                  = 0x74
	ILLEGAL_BYTE_COUNT_IN_BLOCK_TRANSFER               = 0x75
	ILLEGAL_BLOCK_TRANSFER_TYPE                        = 0x76
	BLOCK_TRANSFER_DATA_CHECKSUM_ERROR                 = 0x77
	REQUEST_CORRECTLY_RECEIVED_RESPONSE_PENDING        = 0x78
	INCORRECT_BYTE_COUNT_DURING_BLOCK_TRANSFER         = 0x79
	SERVICE_NOT_SUPPORTED_IN_ACTIVE_DIAGNOSTIC_SESSION = 0x80
)

var (
	ErrGeneralReject                                = &KWP2000Error{GENERAL_REJECT, "General reject"}
	ErrServiceNotSupported                          = &KWP2000Error{SERVICE_NOT_SUPPORTED, "Service not supported"}
	ErrSubFunctionNotSupportedOrInvalidFormat       = &KWP2000Error{SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT, "Sub-function not supported or invalid format"}
	ErrBusyRepeatRequest                            = &KWP2000Error{BUSY_REPEAT_REQUEST, "Busy, repeat request"}
	ErrConditionsNotCorrectOrRequestSequenceError   = &KWP2000Error{CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR, "Conditions not correct or request sequence error"}
	ErrRoutineNotCompleteOrServiceInProgress        = &KWP2000Error{ROUTINE_NOT_COMPLETE_OR_SERVICE_IN_PROGRESS, "Routine not completed or service in progress"}
	ErrRequestOutOfRange                            = &KWP2000Error{REQUEST_OUT_OF_RANGE, "Request out of range or session dropped"}
	ErrSecurityAccessDeniedOrRequested              = &KWP2000Error{SECURITY_ACCESS_DENIED_OR_REQUESTED, "Security access denied"}
	ErrInvalidKey                                   = &KWP2000Error{INVALID_KEY, "Invalid key supplied"}
	ErrExceedNumberOfAttempts                       = &KWP2000Error{EXCEED_NUMBER_OF_ATTEMPTS, "Exceeded number of attempts to get security access"}
	ErrRequiredTimeDelayNotExpired                  = &KWP2000Error{REQUIRED_TIME_DELAY_NOT_EXPIRED, "Required time delay not expired, you cannot gain security access at this moment"}
	ErrDownloadNotAccepted                          = &KWP2000Error{DOWNLOAD_NOT_ACCEPTED, "Download (PC -> ECU) not accepted"}
	ErrImproperDownloadType                         = &KWP2000Error{IMPROPER_DOWNLOAD_TYPE, "Improper download (PC -> ECU) type"}
	ErrCannotDownloadToSpecifiedAddress             = &KWP2000Error{CANNOT_DOWNLOAD_TO_SPECIFIED_ADDRESS, "Unable to download (PC -> ECU) to specified address"}
	ErrCannotDownloadNumberOfBytesRequested         = &KWP2000Error{CANNOT_DOWNLOAD_NUMBER_OF_BYTES_REQUESTED, "Unable to download (PC -> ECU) number of bytes requested"}
	ErrUploadNotAccepted                            = &KWP2000Error{UPLOAD_NOT_ACCEPTED, "Upload (ECU -> PC) not accepted"}
	ErrImproperUploadType                           = &KWP2000Error{IMPROPER_UPLOAD_TYPE, "Improper upload (ECU -> PC) type"}
	ErrCannotUploadFromSpecifiedAddress             = &KWP2000Error{CANNOT_UPLOAD_FROM_SPECIFIED_ADDRESS, "Unable to upload (ECU -> PC) for specified address"}
	ErrCannotUploadNumberOfBytesRequested           = &KWP2000Error{CANNOT_UPLOAD_NUMBER_OF_BYTES_REQUESTED, "Unable to upload (ECU -> PC) number of bytes requested"}
	ErrTransferSuspended                            = &KWP2000Error{TRANSFER_SUSPENDED, "Transfer suspended"}
	ErrTransferAborted                              = &KWP2000Error{TRANSFER_ABORTED, "Transfer aborted"}
	ErrIllegalAddressInBlockTransfer                = &KWP2000Error{ILLEGAL_ADDRESS_IN_BLOCK_TRANSFER, "Illegal address in block transfer"}
	ErrIllegalByteCountInBlockTransfer              = &KWP2000Error{ILLEGAL_BYTE_COUNT_IN_BLOCK_TRANSFER, "Illegal byte count in block transfer"}
	ErrIllegalBlockTransferType                     = &KWP2000Error{ILLEGAL_BLOCK_TRANSFER_TYPE, "Illegal block transfer type"}
	ErrBlockTransferDataChecksumError               = &KWP2000Error{BLOCK_TRANSFER_DATA_CHECKSUM_ERROR, "Block transfer data checksum error"}
	ErrRequestCorrectlyReceivedResponsePending      = &KWP2000Error{REQUEST_CORRECTLY_RECEIVED_RESPONSE_PENDING, "Response pending"}
	ErrIncorrectByteCountDuringBlockTransfer        = &KWP2000Error{INCORRECT_BYTE_COUNT_DURING_BLOCK_TRANSFER, "Incorrect byte count during block transfer"}
	ErrServiceNotSupportedInActiveDiagnosticSession = &KWP2000Error{SERVICE_NOT_SUPPORTED_IN_ACTIVE_DIAGNOSTIC_SESSION, "Service not supported in current diagnostics session"}
)

type KWP2000Error struct {
	Code byte
	Msg  string
}

func (k *KWP2000Error) Error() string {
	return fmt.Sprintf("%s (0x%02X)", k.Msg, k.Code)
}

func TranslateErrorCode(p byte) error {
	switch p {
	case 0x00:
		return nil
	case GENERAL_REJECT:
		return ErrGeneralReject
	case SERVICE_NOT_SUPPORTED:
		return ErrServiceNotSupported
	case SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT:
		return ErrSubFunctionNotSupportedOrInvalidFormat
	case BUSY_REPEAT_REQUEST:
		return ErrBusyRepeatRequest
	case CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR:
		return ErrConditionsNotCorrectOrRequestSequenceError
	case ROUTINE_NOT_COMPLETE_OR_SERVICE_IN_PROGRESS:
		return ErrRoutineNotCompleteOrServiceInProgress
	case REQUEST_OUT_OF_RANGE:
		return ErrRequestOutOfRange
	case SECURITY_ACCESS_DENIED_OR_REQUESTED:
		return ErrSecurityAccessDeniedOrRequested
	case INVALID_KEY:
		return ErrInvalidKey
	case EXCEED_NUMBER_OF_ATTEMPTS:
		return ErrExceedNumberOfAttempts
	case REQUIRED_TIME_DELAY_NOT_EXPIRED:
		return ErrRequiredTimeDelayNotExpired
	case DOWNLOAD_NOT_ACCEPTED:
		return ErrDownloadNotAccepted
	case IMPROPER_DOWNLOAD_TYPE:
		return ErrImproperDownloadType
	case CANNOT_DOWNLOAD_TO_SPECIFIED_ADDRESS:
		return ErrCannotDownloadToSpecifiedAddress
	case CANNOT_DOWNLOAD_NUMBER_OF_BYTES_REQUESTED:
		return ErrCannotDownloadNumberOfBytesRequested
	case UPLOAD_NOT_ACCEPTED:
		return ErrUploadNotAccepted
	case IMPROPER_UPLOAD_TYPE:
		return ErrImproperUploadType
	case CANNOT_UPLOAD_FROM_SPECIFIED_ADDRESS:
		return ErrCannotUploadFromSpecifiedAddress
	case CANNOT_UPLOAD_NUMBER_OF_BYTES_REQUESTED:
		return ErrCannotUploadNumberOfBytesRequested
	case TRANSFER_SUSPENDED:
		return ErrTransferSuspended
	case TRANSFER_ABORTED:
		return ErrTransferAborted
	case ILLEGAL_ADDRESS_IN_BLOCK_TRANSFER:
		return ErrIllegalAddressInBlockTransfer
	case ILLEGAL_BYTE_COUNT_IN_BLOCK_TRANSFER:
		return ErrIllegalByteCountInBlockTransfer
	case ILLEGAL_BLOCK_TRANSFER_TYPE:
		return ErrIllegalBlockTransferType
	case BLOCK_TRANSFER_DATA_CHECKSUM_ERROR:
		return ErrBlockTransferDataChecksumError
	case REQUEST_CORRECTLY_RECEIVED_RESPONSE_PENDING:
		return ErrRequestCorrectlyReceivedResponsePending
	case INCORRECT_BYTE_COUNT_DURING_BLOCK_TRANSFER:
		return ErrIncorrectByteCountDuringBlockTransfer
	case SERVICE_NOT_SUPPORTED_IN_ACTIVE_DIAGNOSTIC_SESSION:
		return ErrServiceNotSupportedInActiveDiagnosticSession
	default:
		return fmt.Errorf("unknown error %X", p)
	}
}

/*
func TranslateErrorCode2(p byte) error {
	switch p {
	case 0x00:
		//return "Affirmative response"
		return nil
	case GENERAL_REJECT:
		return errors.New("general reject")
	case SERVICE_NOT_SUPPORTED:
		return errors.New("mode not supported")
	case SUBFUNCTION_NOT_SUPPORTED_OR_INVALID_FORMAT:
		return errors.New("sub-function not supported or invalid format")
	case BUSY_REPEAT_REQUEST:
		return errors.New("busy, repeat request")
	case CONDITIONS_NOT_CORRECT_OR_REQUEST_SEQUENCE_ERROR:
		return errors.New("conditions not correct or request sequence error")
	case ROUTINE_NOT_COMPLETE_OR_SERVICE_IN_PROGRESS:
		return errors.New("routine not completed or service in progress")
	case REQUEST_OUT_OF_RANGE:
		return errors.New("request out of range or session dropped")
	case SECURITY_ACCESS_DENIED_OR_REQUESTED:
		return errors.New("security access denied")
	case 0x34:
		return errors.New("security access allowed")
	case INVALID_KEY:
		return errors.New("invalid key supplied")
	case EXCEED_NUMBER_OF_ATTEMPTS:
		return errors.New("exceeded number of attempts to get security access")
	case REQUIRED_TIME_DELAY_NOT_EXPIRED:
		return errors.New("required time delay not expired, you cannot gain security access at this moment")
	case DOWNLOAD_NOT_ACCEPTED:
		return errors.New("download (PC -> ECU) not accepted")
	case IMPROPER_DOWNLOAD_TYPE:
		return errors.New("improper download (PC -> ECU) type")
	case CANNOT_DOWNLOAD_TO_SPECIFIED_ADDRESS:
		return errors.New("unable to download (PC -> ECU) to specified address")
	case CANNOT_DOWNLOAD_NUMBER_OF_BYTES_REQUESTED:
		return errors.New("unable to download (PC -> ECU) number of bytes requested")
	case 0x44:
		return errors.New("ready for download")
	case UPLOAD_NOT_ACCEPTED:
		return errors.New("upload (ECU -> PC) not accepted")
	case IMPROPER_UPLOAD_TYPE:
		return errors.New("improper upload (ECU -> PC) type")
	case CANNOT_UPLOAD_FROM_SPECIFIED_ADDRESS:
		return errors.New("unable to upload (ECU -> PC) for specified address")
	case CANNOT_UPLOAD_NUMBER_OF_BYTES_REQUESTED:
		return errors.New("unable to upload (ECU -> PC) number of bytes requested")
	case 0x54:
		return errors.New("ready for upload")
	case 0x61:
		return errors.New("normal exit with results available")
	case 0x62:
		return errors.New("normal exit without results available")
	case 0x63:
		return errors.New("abnormal exit with results")
	case 0x64:
		return errors.New("abnormal exit without results")
	case TRANSFER_SUSPENDED:
		return errors.New("transfer suspended")
	case TRANSFER_ABORTED:
		return errors.New("transfer aborted")
	case ILLEGAL_ADDRESS_IN_BLOCK_TRANSFER:
		return errors.New("illegal address in block transfer")
	case ILLEGAL_BYTE_COUNT_IN_BLOCK_TRANSFER:
		return errors.New("illegal byte count in block transfer")
	case ILLEGAL_BLOCK_TRANSFER_TYPE:
		return errors.New("illegal block transfer type")
	case BLOCK_TRANSFER_DATA_CHECKSUM_ERROR:
		return errors.New("block transfer data checksum error")
	case REQUEST_CORRECTLY_RECEIVED_RESPONSE_PENDING:
		return errors.New("response pending")
	case INCORRECT_BYTE_COUNT_DURING_BLOCK_TRANSFER:
		return errors.New("incorrect byte count during block transfer")
	case SERVICE_NOT_SUPPORTED_IN_ACTIVE_DIAGNOSTIC_SESSION:
		return errors.New("service not supported in current diagnostics session")
	default:
		return fmt.Errorf("unknown error %X", p)
	}
}
*/
