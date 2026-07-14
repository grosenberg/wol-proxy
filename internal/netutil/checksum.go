package netutil

// calculateChecksum calculates Internet checksum (RFC 1071)
func calculateChecksum(data []byte) uint16 {
	var sum uint32

	// Sum all 16-bit words
	for idx := 0; idx < len(data); idx += 2 {
		var word uint16
		if idx+1 < len(data) {
			word = uint16(data[idx])<<8 | uint16(data[idx+1])
		} else {
			// Odd length: pad with zero
			word = uint16(data[idx]) << 8
		}
		sum += uint32(word)
	}

	// Add carry bits
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}

	// One's complement
	return ^uint16(sum)
}

// calculateICMPChecksum calculates checksum specifically for ICMP packets
func calculateICMPChecksum(data []byte) uint16 {
	return calculateChecksum(data)
}
