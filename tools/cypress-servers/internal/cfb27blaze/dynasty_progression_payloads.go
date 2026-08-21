package cfb27blaze

import _ "embed"

// These title-static Dynasty responses were extracted from the supplied
// successful week-one capture. They are kept on explicit routes so unknown
// Dynasty commands continue to fail visibly instead of receiving blanket
// success responses.

//go:embed fixtures/dynasty-161-reply.bin
var dynasty161Payload []byte

//go:embed fixtures/dynasty-175-reply.bin
var dynasty175Payload []byte

//go:embed fixtures/dynasty-177-reply.bin
var dynasty177Payload []byte

//go:embed fixtures/dynasty-191-reply.bin
var dynasty191Payload []byte

//go:embed fixtures/dynasty-193-reply.bin
var dynasty193Payload []byte

//go:embed fixtures/dynasty-221-reply.bin
var dynasty221Payload []byte

//go:embed fixtures/dynasty-223-reply.bin
var dynasty223Payload []byte

//go:embed fixtures/dynasty-271-reply.bin
var dynasty271Payload []byte

//go:embed fixtures/dynasty-275-reply.bin
var dynasty275Payload []byte

//go:embed fixtures/dynasty-311-reply.bin
var dynasty311Payload []byte

//go:embed fixtures/dynasty-313-reply.bin
var dynasty313Payload []byte

//go:embed fixtures/dynasty-321-reply.bin
var dynasty321Payload []byte

//go:embed fixtures/dynasty-323-reply.bin
var dynasty323Payload []byte

//go:embed fixtures/dynasty-361-reply.bin
var dynasty361Payload []byte

//go:embed fixtures/dynasty-363-reply.bin
var dynasty363Payload []byte

//go:embed fixtures/dynasty-391-reply.bin
var dynasty391Payload []byte

//go:embed fixtures/dynasty-393-reply.bin
var dynasty393Payload []byte

//go:embed fixtures/dynasty-411-reply.bin
var dynasty411Payload []byte

//go:embed fixtures/dynasty-413-reply.bin
var dynasty413Payload []byte

//go:embed fixtures/dynasty-501-reply.bin
var dynasty501Payload []byte

//go:embed fixtures/dynasty-541-reply.bin
var dynasty541Payload []byte

//go:embed fixtures/dynasty-561-reply.bin
var dynasty561Payload []byte

//go:embed fixtures/dynasty-800-reply.bin
var dynasty800Payload []byte

//go:embed fixtures/dynasty-1131-reply.bin
var dynasty1131Payload []byte

//go:embed fixtures/dynasty-1133-reply.bin
var dynasty1133Payload []byte

//go:embed fixtures/dynasty-1151-reply.bin
var dynasty1151Payload []byte

//go:embed fixtures/dynasty-1251-reply.bin
var dynasty1251Payload []byte

//go:embed fixtures/dynasty-1271-reply.bin
var dynasty1271Payload []byte

//go:embed fixtures/dynasty-1410-reply.bin
var dynasty1410Payload []byte

func capturedDynastyProgressionPayloads() map[route][]byte {
	return map[route][]byte{
		{ComponentBootStatus, 161}:  dynasty161Payload,
		{ComponentBootStatus, 175}:  dynasty175Payload,
		{ComponentBootStatus, 177}:  dynasty177Payload,
		{ComponentBootStatus, 191}:  dynasty191Payload,
		{ComponentBootStatus, 193}:  dynasty193Payload,
		{ComponentBootStatus, 221}:  dynasty221Payload,
		{ComponentBootStatus, 223}:  dynasty223Payload,
		{ComponentBootStatus, 271}:  dynasty271Payload,
		{ComponentBootStatus, 275}:  dynasty275Payload,
		{ComponentBootStatus, 311}:  dynasty311Payload,
		{ComponentBootStatus, 313}:  dynasty313Payload,
		{ComponentBootStatus, 321}:  dynasty321Payload,
		{ComponentBootStatus, 323}:  dynasty323Payload,
		{ComponentBootStatus, 361}:  dynasty361Payload,
		{ComponentBootStatus, 363}:  dynasty363Payload,
		{ComponentBootStatus, 391}:  dynasty391Payload,
		{ComponentBootStatus, 393}:  dynasty393Payload,
		{ComponentBootStatus, 411}:  dynasty411Payload,
		{ComponentBootStatus, 413}:  dynasty413Payload,
		{ComponentBootStatus, 501}:  dynasty501Payload,
		{ComponentBootStatus, 541}:  dynasty541Payload,
		{ComponentBootStatus, 561}:  dynasty561Payload,
		{ComponentBootStatus, 800}:  dynasty800Payload,
		{ComponentBootStatus, 804}:  dynasty800Payload,
		{ComponentBootStatus, 1131}: dynasty1131Payload,
		{ComponentBootStatus, 1133}: dynasty1133Payload,
		{ComponentBootStatus, 1151}: dynasty1151Payload,
		{ComponentBootStatus, 1251}: dynasty1251Payload,
		{ComponentBootStatus, 1271}: dynasty1271Payload,
		{ComponentBootStatus, 1410}: dynasty1410Payload,
	}
}
